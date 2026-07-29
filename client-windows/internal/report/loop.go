package report

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"cyberstalk.me/shared"
)

// maxBackoff is the upper bound on the retry delay. The spec caps backoff at
// about two minutes so a flapping server is neither starved nor hammered.
const maxBackoff = 2 * time.Minute

// Loop drives the report cycle: produce a payload, send it, wait, repeat.
//
// The cycle is deliberately simple — it sends the latest state every period
// and drops failed rounds (no replay). Replaying an old state would pollute
// the server's last_seen with a stale timestamp and could mask a real outage,
// so only the most recent snapshot ever goes out.
//
// All timing goes through the Now and Sleep fields so tests can run the loop
// instantly and assert the backoff sequence. Next produces an already-
// sanitized payload (collect+mapping happen in the caller); the loop only
// stamps ReportedAt, which is display/debug-only on the server.
type Loop struct {
	Client   *Client
	Interval time.Duration
	Next     func() shared.ReportPayload
	Now      func() time.Time
	Sleep    func(ctx context.Context, d time.Duration) error
	Logger   *slog.Logger
}

// Run sends reports until ctx is cancelled. The first send is immediate (so
// the device appears online right away); each subsequent send waits one period.
//
// On a retryable failure the period doubles, capped at maxBackoff and floored
// at Interval — so with the default 10s interval the waits are 10s, 20s, 40s,
// 80s, 120s, 120s… A success resets the period to Interval. A permanent failure
// (ErrPermanent) jumps straight to maxBackoff and stays there.
func (l *Loop) Run(ctx context.Context) error {
	now := l.Now
	if now == nil {
		now = time.Now
	}
	sleep := l.Sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	log := l.Logger
	if log == nil {
		log = slog.Default()
	}

	period := l.Interval
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		p := l.Next()
		p.ReportedAt = now().UTC()
		// app/description are already sanitized by the mapping layer — safe to
		// log at Debug. The raw title is never in the payload.
		log.Debug("activity",
			"app", p.Activity.App,
			"description", p.Activity.Description,
			"idle", p.Activity.Idle,
			"idle_seconds", p.Activity.IdleSeconds,
		)

		err := l.Client.Send(ctx, p)

		var wait time.Duration
		switch {
		case err == nil:
			log.Debug("report accepted")
			wait = l.Interval
			period = l.Interval
		case errors.Is(err, ErrPermanent):
			// Config error: back off straight to the cap and stay there.
			wait = capBackoff(l.Interval)
			period = capBackoff(l.Interval)
			log.Warn("report failed; backing off", "err", err, "next", wait)
		default:
			// Retryable: wait the current period, then double it (capped).
			wait = period
			period = nextBackoff(l.Interval, period)
			log.Warn("report failed; backing off", "err", err, "next", wait)
		}

		if sleepErr := sleep(ctx, wait); sleepErr != nil {
			return sleepErr
		}
	}
}

// sleepCtx waits for d, returning early when ctx is cancelled so Ctrl+C exits
// the loop immediately, even mid-backoff.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// nextBackoff doubles the current period, capped at maxBackoff and never below
// interval — a failure must never make the agent report faster than configured.
func nextBackoff(interval, period time.Duration) time.Duration {
	d := period * 2
	if d > maxBackoff {
		d = maxBackoff
	}
	if d < interval {
		d = interval
	}
	return d
}

// capBackoff is the delay used after a permanent failure: maxBackoff, but never
// below interval (a slow agent stays slow).
func capBackoff(interval time.Duration) time.Duration {
	if maxBackoff < interval {
		return interval
	}
	return maxBackoff
}
