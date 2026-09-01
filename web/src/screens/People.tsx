import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";

const ROLES = [
  "public-read",
  "private-read",
  "public-triage",
  "private-triage",
  "approver",
  "reporting",
] as const;

// Who can see what, and who can decide about it.
//
// Nobody appears here by having authenticated. Access is granted in advance, so
// this is what an administrator decided rather than who has turned up — and
// being recorded grants a role, it does not let anybody in. They still sign in
// through a configured provider (ACC-21, ACC-29).
export function People() {
  const queries = useQueryClient();
  const [identity, setIdentity] = useState("");
  const [provider, setProvider] = useState("");

  const people = useQuery({
    queryKey: ["people"],
    queryFn: async () => unwrap(await api.GET("/v1/people", {})),
  });
  const mode = useQuery({
    queryKey: ["role-mode"],
    queryFn: async () => unwrap(await api.GET("/v1/roles/mode", {})),
  });
  const bindings = useQuery({
    queryKey: ["role-bindings"],
    queryFn: async () => unwrap(await api.GET("/v1/roles/bindings", {})),
  });

  const record = useMutation({
    mutationFn: async (body: { identity: string; provider?: string }) =>
      unwrap(await api.POST("/v1/people", { body })),
    onSuccess: () => {
      setIdentity("");
      setProvider("");
      void queries.invalidateQueries({ queryKey: ["people"] });
    },
  });

  const grant = useMutation({
    mutationFn: async (who: { identity: string; product: string; role: string }) =>
      unwrap(
        await api.POST("/v1/people", {
          body: {
            identity: who.identity,
            // `effective` is the server's answer rather than a request:
            // whether a grant does anything depends on the mode roles come
            // from, which is not this caller's to assert.
            holds: [
              {
                product: who.product,
                role: who.role as (typeof ROLES)[number],
                effective: true,
              },
            ],
          },
        }),
      ),
    onSuccess: () => void queries.invalidateQueries({ queryKey: ["people"] }),
  });

  const withdraw = useMutation({
    mutationFn: async (who: { identity: string; product: string; role: string }) =>
      unwrap(
        await api.DELETE("/v1/people/{identity}/roles/{product}/{role}", {
          params: { path: who },
        }),
      ),
    onSuccess: () => void queries.invalidateQueries({ queryKey: ["people"] }),
  });

  const endSessions = useMutation({
    mutationFn: async (who: { identity: string }) =>
      unwrap(await api.DELETE("/v1/people/{identity}/sessions", { params: { path: who } })),
    onSuccess: () => void queries.invalidateQueries({ queryKey: ["people"] }),
  });

  if (people.isPending) return <p className="hint">Loading…</p>;
  if (people.isError) {
    return <Failed error={people.error} what="The people could not be read." />;
  }

  const rows = people.data?.items ?? [];
  const derived = mode.data?.mode === "group-bound";

  return (
    <>
      <div className="screen-head">
        <h2>People and access</h2>
        <p>Who can see what, and who can decide about it</p>
      </div>

      {record.error != null && (
        <Failed error={record.error} what="That person could not be recorded." />
      )}
      {grant.error != null && <Failed error={grant.error} what="That role could not be granted." />}
      {withdraw.error != null && (
        <Failed error={withdraw.error} what="That role could not be withdrawn." />
      )}

      <div className="card" style={{ marginBottom: 14 }}>
        <div style={{ display: "flex", gap: 12, flexWrap: "wrap", alignItems: "flex-end" }}>
          <div className="field" style={{ margin: 0, flex: 1, minWidth: 200 }}>
            <label htmlFor="identity">Identity from the sign-in provider</label>
            <input
              id="identity"
              type="text"
              value={identity}
              placeholder="ashwin@example.com"
              onChange={(event) => setIdentity(event.target.value)}
            />
            {/* Matched exactly as the provider gives it (ACC-31). Folding case
                here would make two different people one, which is a security
                answer rather than a tidiness one. */}
            <span style={{ fontSize: "var(--step--1)", color: "var(--faint)" }}>
              Exactly as your provider gives it. Capitals matter here.
            </span>
          </div>
          <div className="field" style={{ margin: 0, minWidth: 140 }}>
            <label htmlFor="provider">Provider</label>
            <input
              id="provider"
              type="text"
              value={provider}
              placeholder="github"
              onChange={(event) => setProvider(event.target.value)}
            />
          </div>
          <button
            type="button"
            className="btn"
            disabled={identity.trim() === "" || record.isPending}
            onClick={() =>
              record.mutate({
                identity: identity.trim(),
                provider: provider.trim() || undefined,
              })
            }
          >
            Record this person
          </button>
        </div>
        <p className="reading" style={{ marginTop: 10 }}>
          No account is ever created by somebody turning up. Recording them here grants what you
          name and nothing else — they still sign in through a configured provider.
        </p>
      </div>

      {rows.length === 0 ? (
        <Empty
          title="Nobody is recorded yet."
          detail="Record somebody above to give them a way in."
        />
      ) : (
        <div className="tablewrap">
          <table>
            <thead>
              <tr>
                <th>Person</th>
                <th>Signs in as</th>
                <th>Roles</th>
                <th style={{ width: 220 }}>Grant</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {rows.map((person) => (
                <tr key={person.identity} className="row">
                  <td>
                    <span className="id">{person.display_name || person.identity}</span>
                    {person.admin && <span className="state"> administrator</span>}
                  </td>
                  <td>
                    {(person.signs_in_by ?? []).length === 0 ? (
                      <span style={{ color: "var(--faint)" }}>&mdash;</span>
                    ) : (
                      (person.signs_in_by ?? []).map((door) => (
                        <div key={`${door.provider} ${door.username}`}>
                          <span className="id">{door.username}</span>{" "}
                          <span className="hint">{door.provider}</span>
                        </div>
                      ))
                    )}
                  </td>
                  <td>
                    {(person.holds ?? []).length === 0 ? (
                      <span style={{ color: "var(--faint)" }}>none</span>
                    ) : (
                      (person.holds ?? []).map((held) => (
                        <span
                          key={`${held.product} ${held.role}`}
                          className="chip"
                          style={{ marginRight: 4, opacity: held.effective ? 1 : 0.55 }}
                          title={
                            held.source === "derived"
                              ? "Derived from a group, so it is withdrawn by changing the group"
                              : "Assigned by an administrator"
                          }
                        >
                          {held.role} {held.product}
                          {held.source === "assigned" && (
                            <>
                              {" "}
                              <button
                                type="button"
                                className="linkish"
                                title="Withdraw this role"
                                onClick={() =>
                                  withdraw.mutate({
                                    identity: person.identity ?? "",
                                    product: held.product ?? "",
                                    role: held.role ?? "",
                                  })
                                }
                              >
                                remove
                              </button>
                            </>
                          )}
                        </span>
                      ))
                    )}
                  </td>
                  <td>
                    <Grant
                      disabled={derived}
                      onGrant={(product, role) =>
                        grant.mutate({ identity: person.identity ?? "", product, role })
                      }
                    />
                  </td>
                  <td>
                    {/* Withdrawing a role does not reach a session already
                        open. Ending them is the other half of taking access
                        away and belongs beside it. */}
                    <button
                      type="button"
                      className="linkish"
                      style={{ color: "var(--muted)" }}
                      title="Sign them out everywhere"
                      onClick={() => endSessions.mutate({ identity: person.identity ?? "" })}
                    >
                      End sessions
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {derived && (
        <div className="alert" style={{ marginTop: 12 }}>
          <strong>Roles come from groups in this deployment</strong>
          <br />
          <span>
            What somebody holds is derived from the groups their provider reports, so granting
            one here would be overwritten. Change the bindings below instead.
          </span>
        </div>
      )}

      <div className="card" style={{ marginTop: 16 }}>
        <h3 style={{ marginTop: 0 }}>Where roles come from</h3>
        <p className="reading" style={{ margin: "0 0 10px" }}>
          Either an administrator assigns them directly, or they are derived from the groups a
          sign-in provider reports. Both at once would mean two answers to one question.
        </p>
        <p className="hint" style={{ margin: 0 }}>
          Currently <b>{mode.data?.mode ?? "reading"}</b>
          {(bindings.data?.items ?? []).length > 0 && (
            <>
              {" "}
              with {(bindings.data?.items ?? []).length} group{" "}
              {(bindings.data?.items ?? []).length === 1 ? "binding" : "bindings"}
            </>
          )}
        </p>
        {(bindings.data?.items ?? []).length > 0 && (
          <div className="tablewrap" style={{ marginTop: 10 }}>
            <table>
              <thead>
                <tr>
                  <th>Group</th>
                  <th>Product</th>
                  <th>Role</th>
                </tr>
              </thead>
              <tbody>
                {(bindings.data?.items ?? []).map((binding) => (
                  <tr key={`${binding.group} ${binding.product} ${binding.role}`}>
                    <td>
                      <span className="id">{binding.group}</span>
                    </td>
                    <td>
                      <span className="id">{binding.product}</span>
                    </td>
                    <td>{binding.role}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <Credentials />
    </>
  );
}

// Granting needs a product as well as a role: a role is always held against
// one, never globally, which is what stops "can triage" meaning everything.
function Grant({
  disabled,
  onGrant,
}: {
  disabled: boolean;
  onGrant: (product: string, role: string) => void;
}) {
  const [product, setProduct] = useState("");
  const [role, setRole] = useState<string>(ROLES[0]);
  const products = useQuery({
    queryKey: ["products"],
    queryFn: async () => unwrap(await api.GET("/v1/products", {})),
  });
  const names = (products.data?.items ?? []).map((each) => each.name ?? "");

  return (
    <div style={{ display: "flex", gap: 5, flexWrap: "wrap", alignItems: "center" }}>
      <select
        aria-label="Product"
        style={{ width: "auto" }}
        value={product}
        onChange={(event) => setProduct(event.target.value)}
      >
        <option value="">product</option>
        {names.map((name) => (
          <option key={name} value={name}>
            {name}
          </option>
        ))}
      </select>
      <select
        aria-label="Role"
        style={{ width: "auto" }}
        value={role}
        onChange={(event) => setRole(event.target.value)}
      >
        {ROLES.map((each) => (
          <option key={each} value={each}>
            {each}
          </option>
        ))}
      </select>
      <button
        type="button"
        className="linkish"
        disabled={disabled || product === ""}
        onClick={() => onGrant(product, role)}
      >
        Grant
      </button>
    </div>
  );
}

// Credentials that are not people. A pipeline uploads with a key scoped to
// what it may send to; a person holds tokens for their own scripts, which
// never carry more than the person does (ACC-14, ACC-40).
function Credentials() {
  const queries = useQueryClient();
  const keys = useQuery({
    queryKey: ["keys"],
    queryFn: async () => unwrap(await api.GET("/v1/keys", {})),
  });
  const tokens = useQuery({
    queryKey: ["tokens"],
    queryFn: async () => unwrap(await api.GET("/v1/people/tokens", {})),
  });

  const revokeKey = useMutation({
    mutationFn: async (name: string) =>
      unwrap(await api.DELETE("/v1/keys/{name}", { params: { path: { name } } })),
    onSuccess: () => void queries.invalidateQueries({ queryKey: ["keys"] }),
  });
  const revokeToken = useMutation({
    mutationFn: async (held: { identity: string; name: string }) =>
      unwrap(await api.DELETE("/v1/people/{identity}/tokens/{name}", { params: { path: held } })),
    onSuccess: () => void queries.invalidateQueries({ queryKey: ["tokens"] }),
  });

  const keyRows = keys.data?.items ?? [];
  const tokenRows = tokens.data?.items ?? [];

  return (
    <div className="card" style={{ marginTop: 16 }}>
      <h3 style={{ marginTop: 0 }}>Credentials that are not people</h3>
      <p className="reading" style={{ margin: "0 0 12px" }}>
        A build pipeline uploads with a key scoped to what it may send to. A person can hold
        tokens for their own scripts, which never carry more than the person does.
      </p>

      {keyRows.length === 0 && tokenRows.length === 0 ? (
        <p className="hint" style={{ margin: 0 }}>
          Nothing is issued.
        </p>
      ) : (
        <div className="tablewrap">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Kind</th>
                <th>Scope</th>
                <th>Last used</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {keyRows.map((key) => (
                <tr
                  key={`key ${key.name}`}
                  className="row"
                  style={{ opacity: key.revoked ? 0.55 : 1 }}
                >
                  <td>
                    <span className="id">{key.name}</span>
                  </td>
                  <td>Pipeline key</td>
                  <td>
                    <span className="id">{key.product}</span>
                    <span className="hint">
                      {" "}
                      {key.stream ? key.stream : "any branch"},{" "}
                      {key.variant ? key.variant : "any variant"}
                    </span>
                  </td>
                  <td className="hint">{key.last_used_at || "never"}</td>
                  <td>
                    {key.revoked ? (
                      <span style={{ color: "var(--faint)" }}>revoked</span>
                    ) : (
                      <button
                        type="button"
                        className="linkish"
                        onClick={() => revokeKey.mutate(key.name ?? "")}
                      >
                        Revoke
                      </button>
                    )}
                  </td>
                </tr>
              ))}
              {tokenRows.map((token) => (
                <tr
                  key={`token ${token.owner} ${token.name}`}
                  className="row"
                  style={{ opacity: token.revoked ? 0.55 : 1 }}
                >
                  <td>
                    <span className="id">{token.name}</span>
                  </td>
                  <td>Personal token</td>
                  <td>
                    Whatever <b>{token.owner}</b> can reach
                    {token.product && (
                      <span className="hint">
                        {" "}
                        <span className="id">{token.product}</span> only
                      </span>
                    )}
                  </td>
                  <td className="hint">{token.last_used_at || "never"}</td>
                  <td>
                    {token.revoked ? (
                      <span style={{ color: "var(--faint)" }}>revoked</span>
                    ) : (
                      <button
                        type="button"
                        className="linkish"
                        onClick={() =>
                          revokeToken.mutate({
                            identity: token.owner ?? "",
                            name: token.name ?? "",
                          })
                        }
                      >
                        Revoke
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <p className="reading" style={{ marginTop: 10 }}>
        A secret is shown once when it is made and never again. What is stored is a hash, so a
        leaked backup is not a set of working credentials.
      </p>
    </div>
  );
}
