const caseEndSafetyBoundaryMilliseconds = 1_000;

export function investigationWindowWithEndBoundary(from?: string, to?: string) {
  if (!to) return { incident_from: from, incident_to: to };
  const end = Date.parse(to);
  return {
    incident_from: from,
    incident_to: Number.isFinite(end)
      ? new Date(end + caseEndSafetyBoundaryMilliseconds).toISOString()
      : to,
  };
}
