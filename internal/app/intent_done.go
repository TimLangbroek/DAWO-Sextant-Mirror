package app

import (
	"context"
	"slices"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/observed"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// intentEndedBy names, per one-shot intent, the executor acks that end it:
// success and failure alike, because a standing intent would run again on
// the next beat either way. Lock and wipe are absent on purpose: a lock
// stands until an operator lifts it, and a wipe ends the device.
var intentEndedBy = map[string][]string{
	// The executor's boot-id guard stops a loop within one boot, but the
	// standing intent would reboot the device again on the next one.
	fleet.IntentReboot:      {observed.AckRebooted},
	fleet.IntentDiagnostics: {observed.AckDiagnostics, observed.AckDiagnosticsFailed},
	// The new password rides the escrow field of the same beat (design 0017).
	fleet.IntentRotateAdmin: {observed.AckAdminRotated, observed.AckAdminRotateFailed},
}

// ClearCompletedIntent retires a one-shot intent once the device reports it
// done. The clear is structural (an intent is runtime data, not build input)
// and rides the check-in that carried the ack. Anything else is left alone,
// including an ack for an intent the device no longer has.
func (s *ConfigService) ClearCompletedIntent(ctx context.Context, c observed.CheckIn) error {
	dev, ok := s.Fleet().Devices[c.Tag]
	if !ok || c.Ack == "" || !slices.Contains(intentEndedBy[dev.Intent], c.Ack) {
		return nil
	}
	return s.ApplyStructural(ctx, fleet.ClearDeviceIntent(c.Tag),
		"intent: "+dev.Intent+" completed on "+c.Tag,
		ports.Author{Name: "sextant-agent", Email: "agent@sextant"})
}
