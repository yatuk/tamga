import { NoAccounts } from "@/components/console/auth";

// The console has no sign-in of its own: it reads the proxy with the key
// entered under Settings. The proxy issues no session for a Clerk account, so
// Clerk's form is not shown here: signing in to it opened nothing.
export default function SignInPage() {
  return (
    <NoAccounts title="Sign in">
      There is nothing to sign in to here. The console is open and reads the proxy with the admin key from Settings.
    </NoAccounts>
  );
}
