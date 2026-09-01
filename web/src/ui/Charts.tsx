import {
  Area,
  AreaChart,
  CartesianGrid,
  Cell,
  Line,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

// The three shapes the reporting decisions name, drawn small. They sit inside
// panels rather than on a page of their own, so they carry no title and no
// legend — the panel says what they are, and a reading underneath says what
// they mean, which is the part a number cannot say for itself.

export type Point = {
  at?: string;
  open?: number;
  opened?: number;
  resolved?: number;
  by_severity?: Record<string, number>;
};

const BANDS = [
  { key: "critical", colour: "var(--sev-critical)" },
  { key: "high", colour: "var(--sev-high)" },
  { key: "medium", colour: "var(--sev-medium)" },
  { key: "low", colour: "var(--sev-low)" },
];

const tip = {
  background: "var(--surface)",
  border: "1px solid var(--line)",
  borderRadius: 6,
  fontSize: 12,
  color: "var(--ink)",
};

const axis = { fontSize: 11, fill: "var(--faint)" };

// Counts here run to five figures, and a narrow axis clips them to their last
// two digits — which reads as data rather than as a layout fault. Shortened so
// the width is predictable whatever the numbers do.
function brief(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(value >= 10_000 ? 0 : 1)}k`;
  return String(value);
}

function day(at?: string): string {
  return (at ?? "").slice(5, 10);
}

// New, resolved and open together. Separately they are three numbers.
export function Pace({ points }: { points: Point[] }) {
  const data = points.map((p) => ({
    at: day(p.at),
    Open: p.open ?? 0,
    New: p.opened ?? 0,
    Resolved: p.resolved ?? 0,
  }));
  if (data.length === 0) return null;
  return (
    <ResponsiveContainer width="100%" height={180}>
      <AreaChart data={data} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
        <CartesianGrid strokeOpacity={0.14} vertical={false} />
        <XAxis dataKey="at" tick={axis} tickLine={false} axisLine={false} />
        <YAxis
          tick={axis}
          tickLine={false}
          axisLine={false}
          width={46}
          tickFormatter={brief}
        />
        <Tooltip contentStyle={tip} />
        <Area
          type="monotone"
          dataKey="Open"
          stroke="var(--accent)"
          fill="var(--accent)"
          fillOpacity={0.12}
        />
        <Line type="monotone" dataKey="New" stroke="var(--sev-high)" dot={false} />
        <Line type="monotone" dataKey="Resolved" stroke="var(--ok)" dot={false} />
      </AreaChart>
    </ResponsiveContainer>
  );
}

// The same total, split. A flat line with a rising critical share is getting
// worse, and one line cannot show that.
export function Mix({ points }: { points: Point[] }) {
  const data = points.map((p) => {
    const by = p.by_severity ?? {};
    return {
      at: day(p.at),
      critical: by.critical ?? 0,
      high: by.high ?? 0,
      medium: by.medium ?? 0,
      low: (by.low ?? 0) + (by.negligible ?? 0) + (by.unknown ?? 0),
    };
  });
  if (data.length === 0) return null;
  return (
    <ResponsiveContainer width="100%" height={180}>
      <AreaChart data={data} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
        <CartesianGrid strokeOpacity={0.14} vertical={false} />
        <XAxis dataKey="at" tick={axis} tickLine={false} axisLine={false} />
        <YAxis
          tick={axis}
          tickLine={false}
          axisLine={false}
          width={46}
          tickFormatter={brief}
        />
        <Tooltip contentStyle={tip} />
        {BANDS.map((band) => (
          <Area
            key={band.key}
            type="monotone"
            dataKey={band.key}
            stackId="1"
            stroke={band.colour}
            fill={band.colour}
            fillOpacity={0.5}
          />
        ))}
      </AreaChart>
    </ResponsiveContainer>
  );
}

// What is open right now. A ring on its own says what somebody already knows,
// so the numbers are beside it rather than inside it.
export function Ring({ point }: { point?: Point }) {
  const by = point?.by_severity ?? {};
  const slices = BANDS.map((band) => ({
    name: band.key,
    colour: band.colour,
    value:
      band.key === "low"
        ? (by.low ?? 0) + (by.negligible ?? 0) + (by.unknown ?? 0)
        : (by[band.key] ?? 0),
  })).filter((slice) => slice.value > 0);

  if (slices.length === 0) {
    return <p className="reading">Nothing is open.</p>;
  }

  return (
    <div className="ring">
      <PieChart width={132} height={132}>
        <Pie
          data={slices}
          dataKey="value"
          cx="50%"
          cy="50%"
          innerRadius={40}
          outerRadius={62}
          paddingAngle={1}
          stroke="none"
        >
          {slices.map((slice) => (
            <Cell key={slice.name} fill={slice.colour} />
          ))}
        </Pie>
        <Tooltip contentStyle={tip} />
      </PieChart>
      <ul className="ringkey">
        {slices.map((slice) => (
          <li key={slice.name}>
            <i style={{ background: slice.colour }} />
            {slice.name}
            <span className="n">{slice.value.toLocaleString()}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
