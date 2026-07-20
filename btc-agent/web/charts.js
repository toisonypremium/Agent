export function soakPercent(soak){const elapsed=Number(soak?.elapsed_seconds)||0;return Math.max(0,Math.min(100,elapsed/(14*86400)*100))}
