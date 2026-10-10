package api

import "net/http"

// codeClerkSSOUnavailable is the code of the 501 the Clerk callbacks answer.
const codeClerkSSOUnavailable = "clerk_sso_unavailable"

// handleClerkCallback serves POST /auth/clerk/saml/callback and
// POST /auth/clerk/oidc/callback. It issues no session: there is no Clerk
// sign-in in this proxy.
//
// The handlers that were here could not complete a sign-in and are gone:
//
//   - Nothing started the flow. The callbacks compared a state cookie with a
//     state query parameter, and no handler ever set that cookie, so the
//     only caller that could present a matching pair was one that wrote
//     both itself. The check held nobody back.
//   - What they then asked Clerk's Backend API for (POST /saml/verify,
//     POST /oauth/token with the secret key, GET /me) is not in that API's
//     published reference. With Clerk, SAML and OIDC end at Clerk's own endpoints, and an
//     application learns who signed in by verifying a Clerk session token.
//
// The routes stay so that whatever is still pointed at them gets an answer
// that says so. A Clerk sign-in, when there is one, goes through accountRole
// and issueSession like the GitHub one: an account nobody assigned a role is
// refused unless the operator set a default.
func handleClerkCallback(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error": "Clerk sign-in is not available on this proxy; sign in with GitHub, or use an API key",
		"code":  codeClerkSSOUnavailable,
	})
}
