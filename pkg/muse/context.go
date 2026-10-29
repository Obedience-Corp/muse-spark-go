package muse

import (
	"context"
	"errors"
)

// contextError preserves both the standard cancellation category and a caller's
// WithCancelCause/WithTimeoutCause reason. Context values stay in-process; MSP
// has no implicit propagation of Go context metadata to the native server.
func contextError(ctx context.Context) error {
	err := ctx.Err()
	if err == nil {
		return nil
	}
	cause := context.Cause(ctx)
	if cause == nil || cause == err {
		return err
	}
	return errors.Join(err, cause)
}
