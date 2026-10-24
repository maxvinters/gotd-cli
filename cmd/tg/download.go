package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/output"
)

// downloadResult is the result of `tg download`.
type downloadResult struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// MarshalText renders a short summary.
func (r downloadResult) MarshalText(w io.Writer) error {
	_, err := fmt.Fprintf(w, "downloaded %s (%d bytes)\n", output.EscapeTerminalText(r.Path), r.Size)
	return err
}

// getMessage fetches a single message by id from peer.
func getMessage(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, id int) (*tg.Message, error) {
	var (
		res tg.MessagesMessagesClass
		err error
	)
	if ch, ok := peer.(*tg.InputPeerChannel); ok {
		res, err = api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
			ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: id}},
		})
	} else {
		res, err = api.MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: id}})
	}
	if err != nil {
		return nil, errors.Wrap(err, "get messages")
	}

	msgs, ok := res.AsModified()
	if !ok {
		return nil, errors.Errorf("unexpected messages type %T", res)
	}
	for _, m := range msgs.GetMessages() {
		if msg, ok := m.(*tg.Message); ok && msg.ID == id {
			return msg, nil
		}
	}
	return nil, errors.Errorf("message #%d not found", id)
}

// documentFilename returns the filename attribute of a document, or a fallback.
func documentFilename(doc *tg.Document) string {
	for _, attr := range doc.Attributes {
		if name, ok := attr.(*tg.DocumentAttributeFilename); ok && name.FileName != "" {
			return name.FileName
		}
	}
	return "document_" + strconv.FormatInt(doc.ID, 10)
}

// largestPhotoSize returns the type of the largest available photo size.
func largestPhotoSize(p *tg.Photo) (string, bool) {
	var (
		bestType string
		bestArea int
		found    bool
	)
	for _, s := range p.Sizes {
		switch v := s.(type) {
		case *tg.PhotoSize:
			if area := v.W * v.H; area >= bestArea {
				bestArea, bestType, found = area, v.Type, true
			}
		case *tg.PhotoSizeProgressive:
			if area := v.W * v.H; area >= bestArea {
				bestArea, bestType, found = area, v.Type, true
			}
		}
	}
	return bestType, found
}

// mediaLocation extracts a downloadable file location and default filename from
// message media.
func mediaLocation(m tg.MessageMediaClass) (tg.InputFileLocationClass, string, error) {
	switch v := m.(type) {
	case *tg.MessageMediaDocument:
		doc, ok := v.Document.AsNotEmpty()
		if !ok {
			return nil, "", errors.New("empty document")
		}
		return doc.AsInputDocumentFileLocation(""), documentFilename(doc), nil
	case *tg.MessageMediaPhoto:
		ph, ok := v.Photo.AsNotEmpty()
		if !ok {
			return nil, "", errors.New("empty photo")
		}
		sizeType, ok := largestPhotoSize(ph)
		if !ok {
			return nil, "", errors.New("no downloadable photo size")
		}
		loc := &tg.InputPhotoFileLocation{
			ID:            ph.ID,
			AccessHash:    ph.AccessHash,
			FileReference: ph.FileReference,
			ThumbSize:     sizeType,
		}
		return loc, "photo_" + strconv.FormatInt(ph.ID, 10) + ".jpg", nil
	default:
		return nil, "", errors.Errorf("message has no downloadable media (%T)", m)
	}
}

// resolveOutPath decides the destination path given the user's --out and the
// media's default filename.
func resolveOutPath(out, defaultName string) string {
	if out == "" {
		return defaultName
	}
	if info, err := os.Stat(out); err == nil && info.IsDir() {
		return filepath.Join(out, defaultName)
	}
	return out
}

func validateRemoteFilename(name string) error {
	if name == "" || name == "." || name == ".." || filepath.IsAbs(name) ||
		filepath.VolumeName(name) != "" || strings.ContainsAny(name, `/\\:`) {
		return errors.New("invalid remote filename")
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") || windowsReservedDeviceName(name) {
		return errors.New("invalid remote filename")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("invalid remote filename")
		}
	}
	return nil
}

func windowsReservedDeviceName(name string) bool {
	base := name
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	base = strings.TrimRight(base, " .")
	base = strings.ToUpper(base)
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	runes := []rune(base)
	if len(runes) != 4 || (string(runes[:3]) != "COM" && string(runes[:3]) != "LPT") {
		return false
	}
	switch runes[3] {
	case '1', '2', '3', '4', '5', '6', '7', '8', '9', '¹', '²', '³':
		return true
	default:
		return false
	}
}

func hasTrailingPathSeparator(path string, separator rune) bool {
	if strings.HasSuffix(path, string(separator)) {
		return true
	}
	return separator == '\\' && strings.HasSuffix(path, "/")
}

// saveDownload creates an output file without following or replacing an
// existing file, then streams into the opened file descriptor.
func saveDownload(out, remoteName string, download func(io.WriterAt) error) (result downloadResult, retErr error) {
	if err := validateRemoteFilename(remoteName); err != nil {
		return downloadResult{}, err
	}

	path := resolveOutPath(out, remoteName)
	if out != "" && path == out && hasTrailingPathSeparator(out, filepath.Separator) {
		return downloadResult{}, errors.New("output directory does not exist")
	}
	parent, name := filepath.Dir(path), filepath.Base(path)
	if name == "." || name == string(filepath.Separator) {
		return downloadResult{}, errors.New("invalid output file path")
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return downloadResult{}, errors.Wrap(err, "open output directory")
	}
	defer func() {
		if err := root.Close(); retErr == nil && err != nil {
			result = downloadResult{}
			retErr = errors.Wrap(err, "close output directory")
		}
	}()
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return downloadResult{}, errors.Wrap(err, "create output file")
	}
	defer func() {
		if err := file.Close(); retErr == nil && err != nil {
			result = downloadResult{}
			retErr = errors.Wrap(err, "close downloaded file")
		}
	}()

	if err := download(file); err != nil {
		// Root.Remove is name-based, so it cannot safely distinguish a replacement.
		return downloadResult{}, errors.Wrapf(err, "partial download retained at %s", output.EscapeTerminalText(path))
	}
	info, err := file.Stat()
	if err != nil {
		return downloadResult{}, errors.Wrap(err, "stat downloaded file")
	}
	return downloadResult{Path: path, Size: info.Size()}, nil
}

func (a *app) newDownloadCmd() *cobra.Command {
	var out string

	cmd := &cobra.Command{
		Use:     "download <peer> <message-id>",
		Aliases: []string{"dl"},
		Short:   "Download media from a message",
		GroupID: groupMessaging,
		Long: `Download the media (document, video, audio, photo, ...) attached to a
message. --out may be a file path or a directory; by default the media's own
filename is used in the current directory.`,
		Example: `  tg download @durov 12345
  tg download me 1000 --out ./downloads/
  tg download @channel 42 --out file.bin`,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[1])
			if err != nil {
				return errors.Wrap(err, "message-id must be an integer")
			}

			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				peer, err := resolvePeer(ctx, m, args[0])
				if err != nil {
					return err
				}

				msg, err := getMessage(ctx, api, peer, id)
				if err != nil {
					return err
				}
				media, ok := msg.GetMedia()
				if !ok {
					return errors.Errorf("message #%d has no media", id)
				}
				loc, name, err := mediaLocation(media)
				if err != nil {
					return err
				}
				result, err := saveDownload(out, name, func(dst io.WriterAt) error {
					_, err := downloader.NewDownloader().Download(api, loc).Parallel(ctx, dst)
					return err
				})
				if err != nil {
					return errors.Wrap(err, "download")
				}
				return a.printer.Emit(result)
			})
		},
	}

	cmd.Flags().StringVar(&out, "out", "", "output file or directory (default: media filename in cwd)")
	_ = cmd.MarkFlagFilename("out")

	return cmd
}
