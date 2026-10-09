package webhooks

import "github.com/yatuk/tamga/internal/egress"

// Every request this package sends goes through an egress guard: the probe,
// live delivery to saved destinations and rule notify URLs. The guard itself
// lives in internal/egress so other packages can use it; these names are
// what the rest of the proxy wires alerts with.
type (
	Guard       = egress.Guard
	GuardConfig = egress.Config
)

// NewGuard builds a guard. The zero config is the strict default: https
// only, no internal addresses.
func NewGuard(cfg GuardConfig) *Guard { return egress.New(cfg) }

// The two refusals a caller of the probe can be told about.
var (
	ErrDestinationNotAllowed = egress.ErrNotAllowed
	ErrInsecureScheme        = egress.ErrInsecureScheme
)
