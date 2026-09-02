// What the notification area says, apart from how it draws it.
//
// Here rather than inside the component so both can be tested without a DOM:
// these two are where saying the wrong thing is a defect rather than a matter
// of taste.

// The words the server sends are for a machine to match on; these are for
// somebody to read. A kind this does not know is shown as it arrived — a
// server that grows one before the interface does should leave somebody
// reading something unfamiliar, not a blank row where a notice was.
export function label(kind?: string): string {
  switch (kind) {
    case "assigned":
      return "assigned to you";
    case "sent-back":
      return "sent back";
    case "build-quiet":
      return "not being scanned";
    default:
      return kind ?? "";
  }
}

// The count on the control that opens it.
//
// A number rather than a dot, because "three things" and "something" are
// different amounts of interruption and the number is what decides whether
// somebody opens it now or later. Past what fits, it stops counting rather
// than widening — and a total that cannot be drawn at all reads as the same
// nothing an empty list does.
export function waiting(total: number): string {
  if (!Number.isFinite(total) || total < 1) {
    return "·";
  }
  const whole = Math.floor(total);
  return whole > 99 ? "99+" : String(whole);
}
