// Package pretty implements pretty-print facilities.
package pretty

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// Middleware logs RPC types and timing without exposing request or response data.
func Middleware() telegram.MiddlewareFunc {
	return func(next tg.Invoker) telegram.InvokeFunc {
		return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
			_, _ = fmt.Fprintf(os.Stderr, "→ %T\n", input)
			start := time.Now()
			err := next.Invoke(ctx, input, output)
			elapsed := time.Since(start).Round(time.Millisecond)
			if err != nil {
				code := "unknown"
				if rpcErr, ok := tgerr.As(err); ok {
					code = fmt.Sprint(rpcErr.Code)
				}
				_, _ = fmt.Fprintf(os.Stderr, "← %T (%s) error_code=%s\n", output, elapsed, code)
				return err
			}
			_, _ = fmt.Fprintf(os.Stderr, "← %T (%s)\n", output, elapsed)
			return nil
		}
	}
}
