#!/usr/bin/env python3
"""Event Hunter 設計契約的確定性本機驗證工具。

此工具只讀取 repo 內的 YAML、JSON Schema、fixture 與 traceability，不連線資料庫、Kafka
或網路服務，因此可在開發機與 CI 得到相同結果。
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
from pathlib import Path
from typing import Any, Iterable

import yaml
from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource

from fixture_mapping import validate_mapping_contract


PROJECT_ROOT = Path(__file__).resolve().parents[1]
NON_SOURCE_DIRECTORIES = {".git", "artifacts", "dist", "node_modules"}
# 只有這些 key 代表 OpenAPI operation；parameters、servers 等 path-level key 不可當成 API。
HTTP_METHODS = {"get", "put", "post", "delete", "patch", "head", "options", "trace"}


class ContractError(RuntimeError):
    """代表契約本身有衝突或無法解析，而不是外部服務暫時失敗。"""

    pass


class UniqueKeyLoader(yaml.SafeLoader):
    """拒絕重複 YAML key，避免一般 parser 靜默保留最後一個值。"""

    pass


def construct_unique_mapping(loader: UniqueKeyLoader, node: yaml.MappingNode, deep: bool = False) -> dict[Any, Any]:
    # PyYAML 預設允許相同 key 重複出現；OpenAPI 若發生這種情況可能悄悄遺失一段設定。
    mapping: dict[Any, Any] = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node, deep=deep)
        if key in mapping:
            raise ContractError(f"duplicate YAML key {key!r} at line {key_node.start_mark.line + 1}")
        mapping[key] = loader.construct_object(value_node, deep=deep)
    return mapping


UniqueKeyLoader.add_constructor(
    yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG,
    construct_unique_mapping,
)


def load_yaml(path: Path) -> Any:
    try:
        return yaml.load(path.read_text(encoding="utf-8"), Loader=UniqueKeyLoader)
    except Exception as exc:
        raise ContractError(f"{path.relative_to(PROJECT_ROOT)}: {exc}") from exc


def load_json(path: Path) -> Any:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except Exception as exc:
        raise ContractError(f"{path.relative_to(PROJECT_ROOT)}: {exc}") from exc


def is_project_source(path: Path) -> bool:
    """排除 dependency、build 與測試報告內不屬於 Event Hunter 的文件。"""

    return NON_SOURCE_DIRECTORIES.isdisjoint(path.relative_to(PROJECT_ROOT).parts)


def walk(value: Any) -> Iterable[Any]:
    """深度走訪任意 YAML／JSON 結構，供 $ref 收集共用。"""

    yield value
    if isinstance(value, dict):
        for child in value.values():
            yield from walk(child)
    elif isinstance(value, list):
        for child in value:
            yield from walk(child)


def resolve_json_pointer(document: Any, reference: str, source: Path) -> Any:
    # 依 RFC 6901 處理 ~0 與 ~1，確保 OpenAPI 內部 $ref 指向真實節點。
    if reference == "#":
        return document
    if not reference.startswith("#/"):
        raise ContractError(f"{source.relative_to(PROJECT_ROOT)}: unsupported fragment {reference}")
    current = document
    for raw_part in reference[2:].split("/"):
        part = raw_part.replace("~1", "/").replace("~0", "~")
        if not isinstance(current, dict) or part not in current:
            raise ContractError(f"{source.relative_to(PROJECT_ROOT)}: unresolved reference {reference}")
        current = current[part]
    return current


def validate_references(path: Path, document: Any) -> int:
    """驗證文件內部與相對檔案 $ref；HTTP URL 只視為外部契約，不在此工具連網。"""

    count = 0
    for value in walk(document):
        if not isinstance(value, dict) or not isinstance(value.get("$ref"), str):
            continue
        reference = value["$ref"]
        count += 1
        if reference.startswith("#"):
            resolve_json_pointer(document, reference, path)
            continue
        if reference.startswith(("http://", "https://")):
            continue
        file_part, separator, fragment = reference.partition("#")
        target_path = (path.parent / file_part).resolve()
        if not target_path.is_file():
            raise ContractError(f"{path.relative_to(PROJECT_ROOT)}: missing reference target {reference}")
        if separator:
            target = load_json(target_path) if target_path.suffix == ".json" else load_yaml(target_path)
            resolve_json_pointer(target, f"#{fragment}", target_path)
    return count


def operation_ids(openapi: dict[str, Any]) -> set[str]:
    # operationId 是 traceability 與 Huma handler 的穩定連接點，所以必須唯一。
    result: set[str] = set()
    for path_item in openapi["paths"].values():
        for method, operation in path_item.items():
            if method in HTTP_METHODS and isinstance(operation, dict) and operation.get("operationId"):
                operation_id = operation["operationId"]
                if operation_id in result:
                    raise ContractError(f"duplicate OpenAPI operationId {operation_id}")
                result.add(operation_id)
    return result


def resolve_openapi_parameter(openapi: dict[str, Any], parameter: dict[str, Any]) -> dict[str, Any]:
    reference = parameter.get("$ref")
    if reference:
        resolved = resolve_json_pointer(openapi, reference, PROJECT_ROOT / "openapi.yaml")
        if not isinstance(resolved, dict):
            raise ContractError(f"OpenAPI parameter reference is not an object: {reference}")
        return resolved
    return parameter


def validate_openapi_parameters(openapi: dict[str, Any]) -> None:
    # 同一 operation 不可出現相同 in + name 的參數，否則 client generator 行為可能不一致。
    for path_name, path_item in openapi["paths"].items():
        path_parameters = path_item.get("parameters", [])
        for method, operation in path_item.items():
            if method not in HTTP_METHODS or not isinstance(operation, dict):
                continue
            seen: set[tuple[str, str]] = set()
            for raw_parameter in [*path_parameters, *operation.get("parameters", [])]:
                parameter = resolve_openapi_parameter(openapi, raw_parameter)
                key = (parameter.get("in", ""), parameter.get("name", ""))
                if key in seen:
                    raise ContractError(f"duplicate OpenAPI parameter {key} at {method.upper()} {path_name}")
                seen.add(key)


def fixture_events(fixture: dict[str, Any]) -> Iterable[dict[str, Any]]:
    # 支援單一流程 fixture，以及 exclusion-cases.json 這種多案例集合。
    yield from fixture.get("events", [])
    for case in fixture.get("cases", []):
        yield from case.get("events", [])


def validate_fixtures(schema_paths: list[Path], fixture_paths: list[Path]) -> int:
    """將每筆 canonical event 交給其 eventType 對應的 JSON Schema 2020-12 驗證。"""

    schemas = {path.name: load_json(path) for path in schema_paths}
    # 先以每份 Schema 的 $id 建立離線 registry，禁止 validator 嘗試從網路抓取 $ref。
    registry = Registry()
    for schema in schemas.values():
        registry = registry.with_resource(schema["$id"], Resource.from_contents(schema))

    schema_by_event_type = {
        schema["title"]: schema
        for name, schema in schemas.items()
        if name != "canonical-envelope.schema.json"
    }
    count = 0
    trace_correlations: dict[str, str] = {}
    for fixture_path in fixture_paths:
        fixture = load_json(fixture_path)
        for event in fixture_events(fixture):
            event_type = event.get("eventType")
            schema = schema_by_event_type.get(event_type)
            if schema is None:
                raise ContractError(f"{fixture_path.relative_to(PROJECT_ROOT)}: no schema for eventType {event_type}")
            validator = Draft202012Validator(schema, registry=registry, format_checker=FormatChecker())
            errors = sorted(validator.iter_errors(event), key=lambda error: list(error.path))
            if errors:
                details = "; ".join(f"{list(error.path)} {error.message}" for error in errors)
                raise ContractError(f"{fixture_path.relative_to(PROJECT_ROOT)}: {event_type}: {details}")
            trace_id = event.get("traceId")
            correlation_id = event.get("correlationId")
            previous_correlation = trace_correlations.setdefault(trace_id, correlation_id)
            if previous_correlation != correlation_id:
                raise ContractError(
                    f"{fixture_path.relative_to(PROJECT_ROOT)}: traceId {trace_id} is shared by "
                    f"correlations {previous_correlation} and {correlation_id}"
                )
            count += 1
    return count


def validate_pattern_fixtures(pattern_paths: list[Path]) -> None:
    # Pattern contract 宣告的正向與反向 fixture 都必須存在，避免測試連結失效。
    for pattern_path in pattern_paths:
        pattern = load_yaml(pattern_path)
        fixture_groups = pattern.get("fixtures", {})
        for reference in [*fixture_groups.get("matches", []), *fixture_groups.get("does_not_match", [])]:
            target = (pattern_path.parent / reference).resolve()
            if not target.is_file():
                raise ContractError(f"{pattern_path.relative_to(PROJECT_ROOT)}: missing fixture {reference}")


def validate_journey_profiles(schema_path: Path, profile_paths: list[Path], event_schema_paths: list[Path]) -> int:
    """Validate Journey Profile shape, registry identity, and canonical event-type references."""

    schema = load_json(schema_path)
    Draft202012Validator.check_schema(schema)
    validator = Draft202012Validator(schema)
    known_event_types = {
        load_json(path)["title"]
        for path in event_schema_paths
        if path.name != "canonical-envelope.schema.json"
    }
    profile_ids: set[str] = set()
    active_defaults: list[str] = []
    for profile_path in profile_paths:
        profile = load_yaml(profile_path)
        errors = sorted(validator.iter_errors(profile), key=lambda error: list(error.path))
        if errors:
            details = "; ".join(f"{list(error.path)} {error.message}" for error in errors)
            raise ContractError(f"{profile_path.relative_to(PROJECT_ROOT)}: {details}")

        profile_id = profile["id"]
        if profile_id in profile_ids:
            raise ContractError(f"duplicate Journey Profile id {profile_id}")
        profile_ids.add(profile_id)
        if profile["default"] and profile["status"] == "active":
            active_defaults.append(profile_id)

        milestone_ids = [milestone["id"] for milestone in profile["milestones"]]
        if len(milestone_ids) != len(set(milestone_ids)):
            raise ContractError(f"{profile_id}: duplicate milestone ids")
        anomaly_codes = [rule["code"] for rule in profile["anomaly_rules"]]
        if len(anomaly_codes) != len(set(anomaly_codes)):
            raise ContractError(f"{profile_id}: duplicate anomaly codes")

        referenced_event_types: set[str] = set()
        for milestone in profile["milestones"]:
            referenced_event_types.update(milestone["expected_event_types"])
            for rule in milestone["state_rules"]:
                referenced_event_types.update(rule["when_any_event_types"])
                referenced_event_types.update(rule.get("unless_any_event_types", []))
        for rule in profile["journey_state_rules"]:
            referenced_event_types.update(rule["when_any_event_types"])
            referenced_event_types.update(rule.get("unless_any_event_types", []))
        for rule in profile["anomaly_rules"]:
            referenced_event_types.update(rule["trigger_event_types"])
            referenced_event_types.update(rule["required_any_event_types"])
            referenced_event_types.update(rule["evidence_event_types"])
        unknown_event_types = referenced_event_types - known_event_types
        if unknown_event_types:
            raise ContractError(
                f"{profile_id}: references event types without canonical schemas: {sorted(unknown_event_types)}"
            )

    if len(active_defaults) != 1:
        raise ContractError(f"Journey Profiles require exactly one active default, got {active_defaults}")
    return len(profile_paths)


def validate_event_lab_scenarios(contract: dict[str, Any], openapi: dict[str, Any]) -> None:
    """Keep stable S1-S14 identity and Hybrid execution semantics aligned."""

    scenarios = contract.get("scenarios", [])
    catalog_ids = [scenario["id"] for scenario in scenarios]
    expected_ids = [f"S{number}" for number in range(1, 15)]
    if catalog_ids != expected_ids:
        raise ContractError(f"Event Lab IDs must be stable S1-S14 in order, got {catalog_ids}")

    catalog_names = [scenario["name"] for scenario in scenarios]
    api_names = openapi["components"]["schemas"]["ScenarioName"]["enum"]
    if catalog_names != api_names:
        raise ContractError(
            "Event Lab scenario drift: event-lab.yaml scenarios must match "
            "event-lab.openapi.yaml ScenarioName enum in order"
        )

    live_ids = {"S1", "S12", "S13", "S14"}
    if any(
        scenario["execution_mode"] != ("LIVE_SERVICES" if scenario["id"] in live_ids else "LAB_INJECTION")
        for scenario in scenarios
    ):
        raise ContractError(
            "Event Lab Hybrid boundary drift: S1/S12-S14 must be LIVE_SERVICES and S2-S11 LAB_INJECTION"
        )


def validate_generated_pattern_registry() -> None:
    command = [sys.executable, str(PROJECT_ROOT / "scripts" / "generate-pattern-registry.py"), "--check"]
    result = subprocess.run(command, cwd=PROJECT_ROOT, capture_output=True, text=True, check=False)
    if result.returncode != 0:
        detail = result.stdout.strip() or result.stderr.strip() or "generated registry is stale"
        raise ContractError(f"Pattern Registry drift: {detail}")


def validate_generated_journey_registry() -> None:
    command = [sys.executable, str(PROJECT_ROOT / "scripts" / "generate-journey-registry.py"), "--check"]
    result = subprocess.run(command, cwd=PROJECT_ROOT, capture_output=True, text=True, check=False)
    if result.returncode != 0:
        detail = result.stdout.strip() or result.stderr.strip() or "generated registry is stale"
        raise ContractError(f"Journey Profile Registry drift: {detail}")


def validate_traceability(
    openapi: dict[str, Any],
    traceability: dict[str, Any],
    system_openapis: list[dict[str, Any]],
) -> int:
    """確認每個需求都有驗收 feature，且引用的 OpenAPI operationId 確實存在。"""

    available_operations = operation_ids(openapi)
    mapped_features: set[str] = set()
    expected_operations: set[str] = set()
    for requirement in traceability["requirements"]:
        features = requirement.get("acceptance_features", [])
        if not features:
            raise ContractError(f"{requirement['id']}: no acceptance_features")
        mapped_features.update(features)
        expected_operations.update(requirement.get("openapi_operations", []))

    missing_operations = expected_operations - available_operations
    if missing_operations:
        raise ContractError(f"traceability references missing operationIds: {sorted(missing_operations)}")

    available_system_operations: set[str] = set()
    for system_openapi in system_openapis:
        available_system_operations.update(operation_ids(system_openapi))
    expected_system_operations = {
        operation_id
        for requirement in traceability["requirements"]
        for operation_id in requirement.get("system_operations", [])
    }
    missing_system_operations = expected_system_operations - available_system_operations
    if missing_system_operations:
        raise ContractError(
            f"traceability references missing system operationIds: {sorted(missing_system_operations)}"
        )
    missing_features = [path for path in mapped_features if not (PROJECT_ROOT / path).is_file()]
    if missing_features:
        raise ContractError(f"traceability references missing features: {sorted(missing_features)}")
    return len(expected_operations) + len(expected_system_operations)


def validate_fixture_group(schema_path: Path, fixtures: list[tuple[Path, str | None]]) -> int:
    """使用一份 JSON Schema 驗證完整 fixture，或 fixture 中指定名稱的陣列元素。"""

    schema = load_json(schema_path)
    Draft202012Validator.check_schema(schema)
    validator = Draft202012Validator(schema, format_checker=FormatChecker())
    count = 0
    for fixture_path, array_key in fixtures:
        fixture = load_json(fixture_path)
        instances = [fixture] if array_key is None else fixture.get(array_key, [])
        if array_key is not None and not isinstance(instances, list):
            raise ContractError(f"{fixture_path.relative_to(PROJECT_ROOT)}: {array_key} must be an array")
        for index, instance in enumerate(instances):
            errors = sorted(validator.iter_errors(instance), key=lambda error: list(error.path))
            if errors:
                details = "; ".join(f"{list(error.path)} {error.message}" for error in errors)
                location = array_key if array_key is None else f"{array_key}[{index}]"
                raise ContractError(f"{fixture_path.relative_to(PROJECT_ROOT)}: {location}: {details}")
            count += 1
    return count


def validate_requirement_sets(
    project_scope: dict[str, Any],
    traceability: dict[str, Any],
    implementation_plan: dict[str, Any],
) -> None:
    """確認 scope、需求矩陣與工程 DAG 沒有出現不同版本的 MVP 清單。"""

    scope_ids = {item["id"] for item in project_scope["mvp_capabilities"] if item.get("required")}
    traceability_ids = {item["id"] for item in traceability["requirements"]}
    if scope_ids != traceability_ids:
        raise ContractError(
            f"MVP requirement mismatch scope_only={sorted(scope_ids - traceability_ids)} "
            f"traceability_only={sorted(traceability_ids - scope_ids)}"
        )

    tasks = implementation_plan["tasks"]
    task_ids = [task["id"] for task in tasks]
    if len(task_ids) != len(set(task_ids)):
        raise ContractError("implementation-plan contains duplicate task ids")
    task_id_set = set(task_ids)
    allowed_statuses = {"pending", "in_progress", "completed"}
    invalid_statuses = {
        task["id"]: task.get("status") for task in tasks if task.get("status") not in allowed_statuses
    }
    if invalid_statuses:
        raise ContractError(f"implementation-plan has invalid task statuses: {invalid_statuses}")
    covered_requirements = {
        requirement_id for task in tasks for requirement_id in task.get("requirement_ids", [])
    }
    unknown_requirements = covered_requirements - scope_ids
    if unknown_requirements:
        raise ContractError(f"implementation-plan references unknown requirements: {sorted(unknown_requirements)}")
    missing_requirements = scope_ids - covered_requirements
    if missing_requirements:
        raise ContractError(f"implementation-plan does not cover requirements: {sorted(missing_requirements)}")

    dependencies = {task["id"]: task.get("depends_on", []) for task in tasks}
    missing_dependencies = {
        dependency for task_dependencies in dependencies.values() for dependency in task_dependencies
        if dependency not in task_id_set
    }
    if missing_dependencies:
        raise ContractError(f"implementation-plan references missing tasks: {sorted(missing_dependencies)}")

    visiting: set[str] = set()
    visited: set[str] = set()

    def visit(task_id: str) -> None:
        if task_id in visiting:
            raise ContractError(f"implementation-plan dependency cycle includes {task_id}")
        if task_id in visited:
            return
        visiting.add(task_id)
        for dependency in dependencies[task_id]:
            visit(dependency)
        visiting.remove(task_id)
        visited.add(task_id)

    for task_id in task_ids:
        visit(task_id)

    status_by_task = {task["id"]: task["status"] for task in tasks}
    for task in tasks:
        if task["status"] == "completed":
            incomplete_dependencies = [
                dependency for dependency in dependencies[task["id"]]
                if status_by_task[dependency] != "completed"
            ]
            if incomplete_dependencies:
                raise ContractError(
                    f"completed task {task['id']} has incomplete dependencies: {incomplete_dependencies}"
                )


def validate_platform_alignment(
    openapi: dict[str, Any],
    asyncapi: dict[str, Any],
    topic_topology: dict[str, Any],
    outbox_routing: dict[str, Any],
    ingestion_mapping: dict[str, Any],
    state_machine: dict[str, Any],
    project_scope: dict[str, Any],
    service_topology: dict[str, Any],
) -> None:
    """驗證跨檔案最容易漂移的 broker、topic、table、狀態與服務邊界。"""

    external_broker = topic_topology["bootstrap_servers"]["external"][0]
    asyncapi_broker = asyncapi["servers"]["localKafka"]["host"]
    if external_broker != asyncapi_broker:
        raise ContractError(f"Kafka broker mismatch topology={external_broker} asyncapi={asyncapi_broker}")

    channels = asyncapi["channels"]
    known_topics = {topic["name"] for topic in topic_topology["topics"]}
    for topic in topic_topology["topics"]:
        channel_name = topic.get("asyncapi_channel")
        if channel_name is None:
            continue
        if channel_name not in channels or channels[channel_name].get("address") != topic["name"]:
            raise ContractError(f"topic {topic['name']} does not match AsyncAPI channel {channel_name}")

    outbox_services = set()
    for connector in outbox_routing["connectors"]:
        outbox_services.add(connector["service"])
        output_topic = connector["output_topic"]
        configured_topic = connector["config_overrides"]["transforms.outbox.route.topic.replacement"]
        if output_topic not in known_topics:
            raise ContractError(f"outbox connector {connector['id']} references unknown topic {output_topic}")
        if configured_topic != output_topic:
            raise ContractError(
                f"outbox connector {connector['id']} configured topic {configured_topic} != {output_topic}"
            )

    migration_text = "\n".join(
        path.read_text(encoding="utf-8")
        for path in sorted((PROJECT_ROOT / "backend" / "migrations").rglob("*.sql"))
    )

    def migration_creates(kind: str, object_name: str) -> bool:
        """Accept both unqualified and database-qualified ClickHouse DDL names."""

        escaped_name = re.escape(object_name)
        return re.search(
            rf"CREATE\s+(?:MATERIALIZED\s+)?{kind}\s+IF\s+NOT\s+EXISTS\s+(?:[A-Za-z_][A-Za-z0-9_]*\.)?{escaped_name}\b",
            migration_text,
            flags=re.IGNORECASE,
        ) is not None

    for pipeline in ingestion_mapping["pipelines"]:
        unknown_topics = set(pipeline["input"]["topics"]) - known_topics
        if unknown_topics:
            raise ContractError(f"pipeline {pipeline['id']} references unknown topics: {sorted(unknown_topics)}")
        target_table = pipeline["mapping"]["target_table"]
        if not migration_creates("TABLE", target_table):
            raise ContractError(f"pipeline {pipeline['id']} target table has no migration: {target_table}")
        failure_table = pipeline.get("on_validation_failure", {}).get("clickhouse_table")
        if failure_table and not migration_creates("TABLE", failure_table):
            raise ContractError(f"pipeline {pipeline['id']} failure table has no migration: {failure_table}")

    quality_table = load_yaml(PROJECT_ROOT / "contracts" / "quality" / "event-quality-rules.yaml")["worker"]["output_table"]
    if f"CREATE TABLE IF NOT EXISTS {quality_table}" not in migration_text:
        raise ContractError(f"quality output table has no migration: {quality_table}")

    openapi_states = set(openapi["components"]["schemas"]["InvestigationStatus"]["enum"])
    contract_states = {state_machine["initial_state"], *state_machine["terminal_states"]}
    for transition in state_machine["transitions"]:
        contract_states.update(transition["from"])
        contract_states.add(transition["to"])
    if openapi_states != contract_states:
        raise ContractError(
            f"investigation states mismatch openapi_only={sorted(openapi_states - contract_states)} "
            f"state_machine_only={sorted(contract_states - openapi_states)}"
        )

    scoped_services = set(project_scope["event_pipeline"]["demo_services"])
    contract_services = {service["id"] for service in service_topology["services"]}
    if scoped_services != contract_services:
        raise ContractError(
            f"demo services mismatch scope_only={sorted(scoped_services - contract_services)} "
            f"topology_only={sorted(contract_services - scoped_services)}"
        )
    if outbox_services != contract_services:
        raise ContractError(
            f"outbox services mismatch topology_only={sorted(contract_services - outbox_services)} "
            f"outbox_only={sorted(outbox_services - contract_services)}"
        )


def validate_port_registry(
    port_registry: dict[str, Any],
    openapi: dict[str, Any],
    asyncapi: dict[str, Any],
    order_openapi: dict[str, Any],
    service_topology: dict[str, Any],
    event_lab_openapi: dict[str, Any],
    event_lab_contract: dict[str, Any],
) -> int:
    """確保所有本機對外 port 唯一、依賴在前，並與環境變數及主要契約一致。"""

    entries = port_registry["ports"]
    ids = [entry["id"] for entry in entries]
    host_ports = [entry["host_port"] for entry in entries]
    env_vars = [entry["env_var"] for entry in entries]
    if len(ids) != len(set(ids)):
        raise ContractError("port-registry contains duplicate ids")
    if len(host_ports) != len(set(host_ports)):
        raise ContractError("port-registry contains duplicate host ports")
    if len(env_vars) != len(set(env_vars)):
        raise ContractError("port-registry contains duplicate environment variables")

    allowed_statuses = {"active", "reserved", "optional", "development_tool"}
    entry_by_id = {entry["id"]: entry for entry in entries}
    excluded_ports: set[int] = set()
    for excluded in port_registry["policy"].get("excluded_host_ports", []):
        start_text, end_text = str(excluded["range"]).split("-", 1)
        excluded_ports.update(range(int(start_text), int(end_text) + 1))
    for entry in entries:
        if not 28310 <= entry["host_port"] <= 28349:
            raise ContractError(f"port {entry['host_port']} for {entry['id']} is outside reserved 2831x-2834x")
        if entry["host_port"] in excluded_ports:
            raise ContractError(f"port {entry['host_port']} for {entry['id']} is explicitly excluded")
        if entry["status"] not in allowed_statuses:
            raise ContractError(f"port {entry['id']} has invalid status {entry['status']}")
        for dependency in entry["depends_on"]:
            if dependency not in entry_by_id:
                raise ContractError(f"port {entry['id']} references missing dependency {dependency}")
            if entry_by_id[dependency]["host_port"] >= entry["host_port"]:
                raise ContractError(
                    f"port dependency order invalid: {dependency} must be lower than {entry['id']}"
                )

    # .env.example 必須列出 Registry 中每個變數，而且值就是分配的 host port。
    env_example_text = (PROJECT_ROOT / ".env.example").read_text(encoding="utf-8")
    env_keys: set[str] = set()
    env_defaults: dict[str, int] = {}
    for line in env_example_text.splitlines():
        key_match = re.match(r"([A-Z][A-Z0-9_]*)=", line.strip())
        if key_match:
            env_keys.add(key_match.group(1))
        match = re.fullmatch(r"([A-Z][A-Z0-9_]*)=([0-9]+)", line.strip())
        if match:
            env_defaults[match.group(1)] = int(match.group(2))
    for entry in entries:
        actual = env_defaults.get(entry["env_var"])
        if actual != entry["host_port"]:
            raise ContractError(
                f".env.example {entry['env_var']}={actual} does not match port-registry {entry['host_port']}"
            )

    # Active Compose port 的環境變數、預設 host port 與 container port 必須完全相同。
    compose_text = (PROJECT_ROOT / "compose.yaml").read_text(encoding="utf-8")
    compose_env_vars = set(re.findall(r"\$\{([A-Z][A-Z0-9_]*)", compose_text))
    missing_env_vars = sorted(compose_env_vars - env_keys)
    if missing_env_vars:
        raise ContractError(
            f"compose environment variables missing from .env.example: {missing_env_vars}"
        )
    compose_ports = {
        match.group(1): (int(match.group(2)), int(match.group(3)))
        for match in re.finditer(r"\$\{([A-Z][A-Z0-9_]*):-([0-9]+)\}:([0-9]+)", compose_text)
    }
    for entry in entries:
        if entry["status"] != "active":
            continue
        actual = compose_ports.get(entry["env_var"])
        expected = (entry["host_port"], entry["container_port"])
        if actual != expected:
            raise ContractError(
                f"compose port {entry['env_var']}={actual} does not match port-registry {expected}"
            )

    expected_locations = {
        "event-hunter-api": openapi["servers"][0]["url"],
        "redpanda-kafka": asyncapi["servers"]["localKafka"]["host"],
        "demo-order-api": order_openapi["servers"][0]["url"],
        "event-lab-api": event_lab_openapi["servers"][0]["url"],
    }
    for entry_id, location in expected_locations.items():
        expected_port = entry_by_id[entry_id]["host_port"]
        if not re.search(rf":{expected_port}(?:/|$)", location):
            raise ContractError(f"{entry_id} location {location} does not use port {expected_port}")

    service_entry_ids = {
        "order-service": "demo-order-api",
        "payment-service": "demo-payment-service",
        "shipping-service": "demo-shipping-service",
    }
    for service in service_topology["services"]:
        expected_port = entry_by_id[service_entry_ids[service["id"]]]["host_port"]
        if service["http"]["listen_address"] != f":{expected_port}":
            raise ContractError(
                f"service {service['id']} listen address {service['http']['listen_address']} != :{expected_port}"
            )
    event_lab_port = entry_by_id["event-lab-api"]["host_port"]
    if event_lab_contract["service"]["http"]["listen_address"] != f":{event_lab_port}":
        raise ContractError(
            "event-lab listen address does not match reserved event-lab-api port"
        )
    return len(entries)


def main() -> int:
    # 第一階段：所有 YAML／JSON 都必須能解析，並拒絕重複 YAML key。
    yaml_paths = sorted(path for path in PROJECT_ROOT.rglob("*.yaml") if is_project_source(path))
    json_paths = sorted((PROJECT_ROOT / "contracts").rglob("*.json"))
    yaml_documents = {path: load_yaml(path) for path in yaml_paths}
    json_documents = {path: load_json(path) for path in json_paths}

    # 第二階段：解析所有本地 $ref，確保契約不會指向不存在的檔案或節點。
    reference_count = sum(validate_references(path, document) for path, document in yaml_documents.items())
    reference_count += sum(validate_references(path, document) for path, document in json_documents.items())

    # 第三階段：驗證 OpenAPI 與需求追蹤矩陣之間的穩定映射。
    openapi = yaml_documents[PROJECT_ROOT / "openapi.yaml"]
    asyncapi = yaml_documents[PROJECT_ROOT / "contracts" / "asyncapi.yaml"]
    traceability = yaml_documents[PROJECT_ROOT / "requirements" / "traceability.yaml"]
    project_scope = yaml_documents[PROJECT_ROOT / "requirements" / "project-scope.yaml"]
    implementation_plan = yaml_documents[PROJECT_ROOT / "requirements" / "implementation-plan.yaml"]
    system_openapis = [
        yaml_documents[path]
        for path in sorted((PROJECT_ROOT / "contracts" / "demo-services").glob("*.openapi.yaml"))
    ]
    validate_openapi_parameters(openapi)
    mapped_operation_count = validate_traceability(openapi, traceability, system_openapis)
    validate_requirement_sets(project_scope, traceability, implementation_plan)

    # 第四階段：以正式事件 Schema 驗證 fixtures，禁止測試資料另走寬鬆格式。
    schema_paths = sorted((PROJECT_ROOT / "contracts" / "events").glob("*.json"))
    fixture_paths = sorted((PROJECT_ROOT / "contracts" / "fixtures").glob("*.json"))
    event_count = validate_fixtures(schema_paths, fixture_paths)
    validate_pattern_fixtures(sorted((PROJECT_ROOT / "contracts" / "patterns").glob("*.yaml")))
    validate_generated_pattern_registry()
    journey_profile_count = validate_journey_profiles(
        PROJECT_ROOT / "contracts" / "journeys" / "journey-profile.schema.json",
        sorted((PROJECT_ROOT / "contracts" / "journeys").glob("*.yaml")),
        schema_paths,
    )
    validate_generated_journey_registry()
    validate_event_lab_scenarios(
        yaml_documents[PROJECT_ROOT / "contracts" / "event-lab" / "event-lab.yaml"],
        yaml_documents[PROJECT_ROOT / "contracts" / "event-lab" / "event-lab.openapi.yaml"],
    )

    # 第五階段：非 Domain Event 的 webhook 與 processing-attempt fixtures 也必須走正式 Schema。
    attempt_count = validate_fixture_group(
        PROJECT_ROOT / "contracts" / "telemetry" / "event-processing-attempt.schema.json",
        [
            (PROJECT_ROOT / "contracts" / "fixtures" / "processing-attempts.json", "attempts"),
            (PROJECT_ROOT / "contracts" / "fixtures" / "quality-window.json", "attempts"),
        ],
    )
    webhook_count = validate_fixture_group(
        PROJECT_ROOT / "contracts" / "integrations" / "grafana-alert-webhook.schema.json",
        [
            (PROJECT_ROOT / "contracts" / "fixtures" / "grafana-alert-firing.json", None),
            (PROJECT_ROOT / "contracts" / "fixtures" / "grafana-alert-resolved.json", None),
        ],
    )

    # 第六階段：檢查跨契約的 broker、topic、table、狀態機與 Demo 服務拓撲。
    validate_platform_alignment(
        openapi,
        asyncapi,
        yaml_documents[PROJECT_ROOT / "contracts" / "platform" / "topic-topology.yaml"],
        yaml_documents[PROJECT_ROOT / "contracts" / "platform" / "outbox-routing.yaml"],
        yaml_documents[PROJECT_ROOT / "contracts" / "platform" / "ingestion-mapping.yaml"],
        yaml_documents[PROJECT_ROOT / "contracts" / "platform" / "investigation-state-machine.yaml"],
        project_scope,
        yaml_documents[PROJECT_ROOT / "contracts" / "demo-services" / "service-topology.yaml"],
    )
    try:
        fixture_mapping_expression_count = validate_mapping_contract()
    except ValueError as exc:
        raise ContractError(f"fixture mapping adapter: {exc}") from exc
    port_count = validate_port_registry(
        yaml_documents[PROJECT_ROOT / "contracts" / "platform" / "port-registry.yaml"],
        openapi,
        asyncapi,
        system_openapis[0],
        yaml_documents[PROJECT_ROOT / "contracts" / "demo-services" / "service-topology.yaml"],
        yaml_documents[PROJECT_ROOT / "contracts" / "event-lab" / "event-lab.openapi.yaml"],
        yaml_documents[PROJECT_ROOT / "contracts" / "event-lab" / "event-lab.yaml"],
    )

    print(f"OK yaml={len(yaml_paths)} json={len(json_paths)} refs={reference_count}")
    print(
        f"OK mapped_operations={mapped_operation_count} fixture_events={event_count} "
        f"journey_profiles={journey_profile_count} processing_attempts={attempt_count} "
        f"grafana_webhooks={webhook_count} fixture_mapping_expressions={fixture_mapping_expression_count} "
        f"ports={port_count}"
    )
    return 0


if __name__ == "__main__":
    # CI 只需要判斷 exit code；詳細原因寫到 stderr，成功摘要寫到 stdout。
    try:
        raise SystemExit(main())
    except ContractError as exc:
        print(f"CONTRACT VALIDATION FAILED: {exc}", file=sys.stderr)
        raise SystemExit(1)
