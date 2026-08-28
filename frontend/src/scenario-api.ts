import createClient from "openapi-fetch";
import type {
  components as EventLabComponents,
  operations as EventLabOperations,
  paths as EventLabPaths,
} from "./generated/event-lab-openapi";

type Schemas = EventLabComponents["schemas"];

export type ScenarioDefinition = Schemas["ScenarioDefinition"];
export type ScenarioRun = Schemas["ScenarioRun"];
export type ScenarioCheck = Schemas["ScenarioCheck"];
export type ScenarioRunPage = Schemas["ScenarioRunPage"];
export type ScenarioRunFilters = NonNullable<
  EventLabOperations["listScenarioRuns"]["parameters"]["query"]
>;

const runtimeOrigin =
  typeof window === "undefined" ? "http://localhost" : window.location.origin;
const client = createClient<EventLabPaths>({
  baseUrl: new URL("/scenario-api", runtimeOrigin)
    .toString()
    .replace(/\/$/, ""),
  fetch: (request) => globalThis.fetch(request),
});

function unwrap<T>(result: {
  data?: T;
  error?: unknown;
  response: Response;
}): T {
  if (!result.response.ok) {
    const error = result.error as { code?: string } | undefined;
    throw new Error(error?.code || result.response.statusText);
  }
  return result.data as T;
}

export const scenarioApi = {
  catalog: async () => unwrap(await client.GET("/api/v1/scenarios")),
  history: async (filters: ScenarioRunFilters = {}) =>
    unwrap(
      await client.GET("/api/v1/scenario-runs", {
        params: { query: filters },
      }),
    ),
  start: async (scenarioId: string) =>
    unwrap(
      await client.POST("/api/v1/scenario-runs", {
        body: { scenario_id: scenarioId },
      }),
    ),
  run: async (runId: string) =>
    unwrap(
      await client.GET("/api/v1/scenario-runs/{runId}", {
        params: { path: { runId } },
      }),
    ),
};
