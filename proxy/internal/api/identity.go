package api

import (
	"context"
	"net/http"
)

// Actors that are not a person or a key of their own.
const (
	// ActorAdminKey is every caller that presents the static admin key. The
	// key is one shared secret, so the server cannot tell two holders apart.
	ActorAdminKey = "admin-key"
	// ActorDevMode is every caller when TAMGA_DEV_MODE lets requests through
	// without a credential.
	ActorDevMode = "dev-mode"
	// actorUnknown stands in where no identity was established. adminAuth
	// always establishes one, so this is seen only by a handler reached
	// without it.
	actorUnknown = "anonymous"

	actorAPIKeyPrefix = "apikey:"
	actorUserPrefix   = "user:"
)

// identity is who a management request was authenticated as. adminAuth
// derives it from the credential and nothing else: no header or body field
// the client writes takes part.
type identity struct {
	actor string
	role  string
}

type identityKey struct{}

// withIdentity returns r with the verified actor and role in its context.
func withIdentity(r *http.Request, actor, role string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), identityKey{}, identity{actor: actor, role: role}))
}

// ActorFrom returns the verified actor of a management request, or "" when
// the context carries none:
//
//	admin-key        the static admin key
//	apikey:<id>      a scoped API key, by the id of its record (never the secret)
//	user:<subject>   a signed-in user, by the subject of the verified session token
//	dev-mode         dev mode, where no credential is asked for
func ActorFrom(ctx context.Context) string {
	id, _ := ctx.Value(identityKey{}).(identity)
	return id.actor
}

// RoleFrom returns the role the credential resolved to (admin, analyst or
// viewer), or "" when the context carries none. A caller without a role is
// not an admin.
func RoleFrom(ctx context.Context) string {
	id, _ := ctx.Value(identityKey{}).(identity)
	return id.role
}

// sharedActor reports whether an actor stands for more than one possible
// person, so that two requests from it cannot count as two people.
func sharedActor(actor string) bool {
	switch actor {
	case ActorAdminKey, ActorDevMode, actorUnknown, "", actorAPIKeyPrefix, actorUserPrefix:
		return true
	}
	return false
}

// actorFromRequest returns the verified actor for audit entries and for
// authorship. It reads the request context that adminAuth filled; headers
// such as X-Tamga-Actor or X-Tamga-User-Id are not consulted.
func actorFromRequest(r *http.Request) string {
	if a := ActorFrom(r.Context()); a != "" {
		return a
	}
	return actorUnknown
}
