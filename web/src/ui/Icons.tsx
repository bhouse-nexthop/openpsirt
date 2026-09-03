// The rail's and the bar's icons: stroke paths on a 24-unit grid, the
// mockup's, so a screen is found by its shape as well as its word.

const PATHS: Record<string, string> = {
  home: '<path d="M3 11 12 4l9 7"/><path d="M5 10v10h14V10"/>',
  inbox: '<path d="M4 4h16v16H4z"/><path d="M4 14h5l1.5 2h3L15 14h5"/>',
  nobody:
    '<circle cx="10" cy="8" r="3.5"/><path d="M3.5 20a6.5 6.5 0 0 1 13 0"/><path d="m17 8 4 4m0-4-4 4"/>',
  people:
    '<circle cx="9" cy="8" r="3.5"/><path d="M2.5 20a6.5 6.5 0 0 1 13 0"/><path d="M16 4.5a3.5 3.5 0 0 1 0 7"/><path d="M21.5 20a6.5 6.5 0 0 0-4.5-6.2"/>',
  bug: '<path d="M9 9V7a3 3 0 0 1 6 0v2"/><rect x="7" y="9" width="10" height="11" rx="5"/><path d="M3 13h4M17 13h4M4 19l3-2M20 19l-3-2M5 7l3 2M19 7l-3 2"/>',
  tree: '<circle cx="6" cy="5" r="2"/><circle cx="6" cy="19" r="2"/><circle cx="18" cy="12" r="2"/><path d="M6 7v10M8 5.5c4 0 6 2 8 5M8 18.5c4 0 6-2 8-5"/>',
  scan: '<circle cx="12" cy="12" r="8"/><circle cx="12" cy="12" r="3"/><path d="M12 4v2M12 18v2M4 12h2M18 12h2"/>',
  compare:
    '<rect x="3" y="4" width="7" height="16" rx="1.5"/><rect x="14" y="4" width="7" height="16" rx="1.5"/><path d="M10 12h4"/>',
  box: '<path d="m12 3 8 4.5v9L12 21l-8-4.5v-9z"/><path d="M4 7.5 12 12l8-4.5M12 12v9"/>',
  branch:
    '<circle cx="6" cy="5" r="2"/><circle cx="6" cy="19" r="2"/><circle cx="18" cy="8" r="2"/><path d="M6 7v10M18 10c0 4-4 4-8 5"/>',
  layers: '<path d="m12 4 9 5-9 5-9-5z"/><path d="m3 14 9 5 9-5"/>',
  users:
    '<circle cx="9" cy="8" r="3.5"/><path d="M2.5 20a6.5 6.5 0 0 1 13 0"/><path d="M16 4.5a3.5 3.5 0 0 1 0 7"/><path d="M21.5 20a6.5 6.5 0 0 0-4.5-6.2"/>',
  sliders:
    '<path d="M4 7h10M18 7h2M4 17h4M12 17h8"/><circle cx="16" cy="7" r="2"/><circle cx="10" cy="17" r="2"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
  bell: '<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10 21h4"/>',
  upload: '<path d="M12 16V4M6 10l6-6 6 6"/><path d="M4 20h16"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  triage: '<path d="M4 7h16M4 12h10M4 17h7"/><path d="m16 15 2 2 4-4"/>',
  shield:
    '<path fill="currentColor" stroke="none" d="M12 2.2 4.6 5.9v5.4c0 4.6 3.1 8.4 7.4 10.5 4.3-2.1 7.4-5.9 7.4-10.5V5.9Z" opacity=".35"/><path fill="currentColor" stroke="none" d="M12 4.2 6.4 7v4.3c0 3.5 2.3 6.5 5.6 8.2Z"/>',
};

export function Icon({ name, size }: { name: string; size?: number }) {
  const markup = PATHS[name] ?? "";
  return (
    <svg
      viewBox="0 0 24 24"
      aria-hidden="true"
      width={size}
      height={size}
      // Fixed markup from the table above, never from anything typed.
      dangerouslySetInnerHTML={{ __html: markup }}
    />
  );
}
