import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Failed } from "../ui/Failed";

type Provider = { name: string; path: string };

// The one screen somebody sees before they hold anything. It offers the ways
// in this deployment has configured and says nothing else — which of them a
// given person can use is not knowable until they have used it, and guessing
// out loud would tell a stranger who has an account here.
export function SignIn() {
  const providers = useQuery({
    queryKey: ["providers"],
    retry: false,
    queryFn: async () => unwrap(await api.GET("/v1/sign-in", {})),
  });

  return (
    <div className="flex min-h-dvh items-center justify-center px-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex flex-col items-center gap-3">
          <img src="/brand/logo.svg" alt="OpenPSIRT" className="h-10" />
          <p className="text-sm text-[var(--muted)]">Sign in to continue</p>
        </div>

        {providers.isPending && <p className="text-center text-sm text-[var(--muted)]">Loading…</p>}

        {providers.isError && (
          <Failed error={providers.error} what="The ways in could not be read." />
        )}

        {providers.data && (providers.data.items ?? []).length === 0 && (
          <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-4 py-6 text-center">
            <p className="text-sm font-medium">No way in is configured.</p>
            <p className="mt-1 text-sm text-[var(--muted)]">
              An operator configures a sign-in provider before anybody can sign in.
            </p>
          </div>
        )}

        <div className="flex flex-col gap-2">
          {(providers.data?.items ?? []).map((each: Provider) => (
            <a
              key={each.name}
              href={each.path}
              className="rounded-lg bg-[var(--accent)] px-4 py-2.5 text-center text-sm font-medium text-white hover:opacity-90"
            >
              Continue with {each.name}
            </a>
          ))}
        </div>
      </div>
    </div>
  );
}
