import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { label, waiting } from "./notices";

// What is waiting on you, with the count on the way in.
//
// Everyone has one, and what appears differs by what they hold rather than by
// which feature they were given: work arriving, a claim sent back, an approval
// an edit withdrew, or — for an administrator — that the tool itself is unwell.
//
// It works with nothing configured, which is the point. A deployment that
// never set up mail would otherwise send every operational alert into a void,
// and the operator who has not configured anything is exactly the one who
// needs telling.
export function Notices() {
  const [open, setOpen] = useState(false);
  const queries = useQueryClient();

  const mine = useQuery({
    queryKey: ["notifications"],
    queryFn: async () => unwrap(await api.GET("/v1/notifications", {})),
    // Asked for again on a cadence, because most of what lands here is put
    // there by something other than this browser: a scan finishing, somebody
    // else assigning work, a build going quiet.
    refetchInterval: 60_000,
  });
  const total = mine.data?.total ?? 0;
  const items = mine.data?.items ?? [];

  const forget = useMutation({
    mutationFn: async (id: number) =>
      api.DELETE("/v1/notifications/{id}", { params: { path: { id } } }),
    onSuccess: () => void queries.invalidateQueries({ queryKey: ["notifications"] }),
  });
  const forgetAll = useMutation({
    mutationFn: async () => api.DELETE("/v1/notifications", {}),
    onSuccess: () => void queries.invalidateQueries({ queryKey: ["notifications"] }),
  });

  return (
    <div className="notices">
      <button
        type="button"
        className="who noticebell"
        aria-expanded={open}
        aria-label={total > 0 ? `${total} waiting on you` : "Nothing waiting on you"}
        title={total > 0 ? `${total} waiting on you` : "Nothing waiting on you"}
        onClick={() => setOpen(!open)}
      >
        {waiting(total)}
      </button>

      {open && (
        <div className="noticelist" role="dialog" aria-label="What is waiting on you">
          <header>
            <b>Waiting on you</b>
            {total > 0 && (
              <button
                type="button"
                className="linkish"
                onClick={() => forgetAll.mutate()}
                style={{ marginLeft: "auto" }}
              >
                Clear all
              </button>
            )}
          </header>

          {items.length === 0 ? (
            <p className="hint" style={{ margin: 0 }}>Nothing is waiting on you.</p>
          ) : (
            <ul>
              {items.map((notice) => (
                <li key={notice.id}>
                  <div>
                    {/* A condition says so, because acknowledging one hides it
                        rather than resolving it: the thing it is about is
                        still true and will stop being said when that changes,
                        not when somebody clicks. */}
                    <span className={`noticekind${notice.lifetime === "condition" ? " holds" : ""}`}>
                      {label(notice.kind)}
                    </span>
                    {notice.link ? (
                      <Link to={notice.link} onClick={() => setOpen(false)}>{notice.body}</Link>
                    ) : (
                      <span>{notice.body}</span>
                    )}
                    <span className="when">{notice.at?.slice(0, 10)}</span>
                  </div>
                  <button
                    type="button"
                    className="linkish"
                    title={
                      notice.lifetime === "condition"
                        ? "Hide it. It will come back if it stops and starts again"
                        : "Take it off your list"
                    }
                    onClick={() => notice.id && forget.mutate(notice.id)}
                  >
                    ×
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
