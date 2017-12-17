package stream

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx"
	"github.com/pkg/errors"
)

// EventListener interface
type EventListener interface {
	Run(ctx context.Context) (err error)
}

// DefaultEventListener object
type DefaultEventListener struct {
	EventListener
	handler   EventHandler
	replicate *Replicate

	consumedWalPosition uint64
}

// NewDefaultEventListener simple event listener mock
func NewDefaultEventListener(replicate *Replicate, handler EventHandler) EventListener {
	return &DefaultEventListener{
		handler:   handler,
		replicate: replicate,
	}
}

// Run start listener execution
func (d *DefaultEventListener) Run(ctx context.Context) (err error) {

	err = run(ctx,
		d.replicate.conn,
		d.handler,
		d.replicate.config.Streaming.SendStandByStatusPeriod,
		d.replicate.config.Streaming.WaitMessageTimeout,
		&d.consumedWalPosition,
	)

	return errors.WithStack(err)
}

func run(ctx context.Context,
	conn *pgx.ReplicationConn,
	handler EventHandler,
	statusPeriod, messageTimeout time.Duration,
	consumedWalPosition *uint64) (err error) {

	standByStatusTicker := time.NewTicker(statusPeriod)
	defer standByStatusTicker.Stop()

	for {
		select {
		case <-standByStatusTicker.C:

			if err = sendStandByStatus(conn, consumedWalPosition); err != nil {
				return errors.WithStack(err)
			}

		default:
			runContext, cancel := context.WithTimeout(ctx, messageTimeout)
			message, err := conn.WaitForReplicationMessage(runContext)
			cancel()

			if ignore, err := filterError(message, handler, err); err != nil {
				return errors.WithStack(err)
			} else if ignore {
				continue
			}

			if isHeartbeat(message) {
				handler.Heartbeat(message.ServerHeartbeat)
				continue
			}

			if isMessage(message) {
				handler.Message(message.WalMessage)
				continue

			}

			handler.Weird(message, err)
		}
	}
}

func filterError(message *pgx.ReplicationMessage, handler EventHandler, inErr error) (ignore bool, outErr error) {

	if isTimeout(inErr) {
		handler.WaitTimeout()
		return true, nil
	}

	if isCancel(inErr) {
		return false, errors.Wrap(inErr, "canceled context")
	}

	if isEOF(inErr) {
		handler.EOF()
		return false, errors.Wrap(inErr, "end of postgres stream messages")
	}

	if isWeird(message, inErr) {
		handler.Weird(message, inErr)
		return true, nil
	}

	return false, nil
}

func sendStandByStatus(conn *pgx.ReplicationConn, consumedWalPosition *uint64) (err error) {

	var (
		status   *pgx.StandbyStatus
		position = atomic.LoadUint64(consumedWalPosition)
	)

	if status, err = pgx.NewStandbyStatus(position); err != nil {
		return errors.Wrapf(err, "create new standby status object failed, position: %d", position)
	}

	err = conn.SendStandbyStatus(status)
	if err == nil {
		fmt.Printf("[listener] send standby status, position: %d\n", position)
	}
	return errors.Wrapf(err, "send stand by status failed, position: %d", position)
}

func isHeartbeat(m *pgx.ReplicationMessage) bool {
	return m.WalMessage == nil && m.ServerHeartbeat != nil
}
func isMessage(m *pgx.ReplicationMessage) bool          { return m.WalMessage != nil && m.ServerHeartbeat == nil }
func isEOF(err error) bool                              { return err != nil && err.Error() == "EOF" }
func isTimeout(err error) bool                          { return err == context.DeadlineExceeded }
func isCancel(err error) bool                           { return err == context.Canceled }
func isWeird(m *pgx.ReplicationMessage, err error) bool { return (m == nil && err == nil) || err != nil }
