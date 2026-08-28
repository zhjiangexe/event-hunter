import { describe, expect, it } from "vitest";
import { parseInvestigationListQuery } from "./investigation-list-query";

describe("parseInvestigationListQuery", () => {
  it("restores compound filters and stable sorting", () => {
    const state = parseInvestigationListQuery(
      "?query=%20MANUAL-4002%20&status=INVESTIGATING&severity=HIGH&priority=P0&assignee=%20shipping-oncall%20&tag=urgent&correlation_id=ORDER-2001&sort_by=updated_at&sort_order=asc",
    );

    expect(state.filters).toEqual({
      query: "MANUAL-4002",
      status: "INVESTIGATING",
      severity: "HIGH",
      priority: "P0",
      assignee: "shipping-oncall",
      tag: "urgent",
      correlation_id: "ORDER-2001",
      sort_by: "updated_at",
      sort_order: "asc",
    });
  });

  it("drops unknown enums and uses the documented default sort", () => {
    const state = parseInvestigationListQuery(
      "?status=UNKNOWN&severity=BLOCKER&priority=P9&sort_by=severity&sort_order=sideways",
    );

    expect(state.filters).toEqual({
      query: undefined,
      status: undefined,
      severity: undefined,
      priority: undefined,
      assignee: undefined,
      tag: undefined,
      correlation_id: undefined,
      sort_by: "created_at",
      sort_order: "desc",
    });
  });
});
