const LEVELS = ['NONE', 'FIRST_QUARTILE', 'SECOND_QUARTILE', 'THIRD_QUARTILE', 'FOURTH_QUARTILE'];

export function currentDayStyle(count, entry, palette, fallbackLevel) {
  if (count === 0) return { level: 0, fill: palette[0] };
  const actual = LEVELS.indexOf(entry?.contributionLevel);
  const level = actual >= 0 ? actual : fallbackLevel;
  const fill = /^#[a-f\d]{6}$/i.test(entry?.color || '') ? entry.color : palette[level];
  return { level, fill };
}
