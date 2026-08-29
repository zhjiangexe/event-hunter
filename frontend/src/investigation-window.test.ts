import { describe, expect, it } from "vitest";
import { investigationWindowWithEndBoundary } from "./investigation-window";

describe("investigationWindowWithEndBoundary", () => {
  it("adds one second so a case includes events at the tail of the selected second", () => {
    expect(
      investigationWindowWithEndBoundary(
        "2026-08-29T04:38:51.176Z",
        "2026-08-29T04:38:56.091Z",
      ),
    ).toEqual({
      incident_from: "2026-08-29T04:38:51.176Z",
      incident_to: "2026-08-29T04:38:57.091Z",
    });
  });

  it("keeps missing or invalid optional bounds unchanged", () => {
    expect(investigationWindowWithEndBoundary()).toEqual({
      incident_from: undefined,
      incident_to: undefined,
    });
    expect(
      investigationWindowWithEndBoundary("invalid-from", "invalid-to"),
    ).toEqual({
      incident_from: "invalid-from",
      incident_to: "invalid-to",
    });
  });
});
