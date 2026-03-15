#!/usr/bin/env python3

import json
import os
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
ARTIFACTS_DIR = ROOT / "artifacts" / "docs"
STATIC_OPENAPI = ROOT / "docs" / "static" / "openapi.json"
MERGED_OPENAPI = ARTIFACTS_DIR / "openapi.swagger.json"

MODULE_TAGS = {
    "dns": "DNS",
    "gateway": "Gateway",
    "pqc": "PQC",
    "release": "Release",
    "tokenomics": "Tokenomics",
}

MODULE_DESCRIPTIONS = {
    "dns": "DNS module REST and tx endpoints.",
    "gateway": "Gateway module REST and tx endpoints.",
    "pqc": "PQC module REST and tx endpoints.",
    "release": "Release module REST and tx endpoints.",
    "tokenomics": "Tokenomics module REST and tx endpoints.",
}


def load_json(path: Path) -> dict:
    with path.open() as fh:
        return json.load(fh)


def dump_json(path: Path, data: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w") as fh:
        json.dump(data, fh, separators=(",", ":"), sort_keys=True)


def module_from_path(path: Path) -> str:
    rel = path.relative_to(ARTIFACTS_DIR)
    if len(rel.parts) < 2 or rel.parts[0] != "lumen":
        raise ValueError(f"unexpected OpenAPI fragment path: {path}")
    return rel.parts[1]


def prefixed_operation_id(module: str, operation_id: str | None) -> str | None:
    if not operation_id:
        return operation_id
    prefix = MODULE_TAGS.get(module, module.title()).replace(" ", "")
    return f"{prefix}_{operation_id}"


def merge_specs(specs: list[tuple[str, Path, dict]], version: str) -> dict:
    merged = {
        "swagger": "2.0",
        "info": {
            "title": "HTTP API Console",
            "description": "Chain lumen REST API",
            "contact": {"name": "lumen"},
            "version": version or "unversioned",
        },
        "consumes": ["application/json"],
        "produces": ["application/json"],
        "paths": {},
        "definitions": {},
        "tags": [],
    }

    seen_tags = set()
    for module, _, _ in specs:
        tag = MODULE_TAGS.get(module, module.title())
        if tag in seen_tags:
            continue
        seen_tags.add(tag)
        merged["tags"].append(
            {
                "name": tag,
                "description": MODULE_DESCRIPTIONS.get(module, f"{tag} module endpoints."),
            }
        )

    for module, path, data in specs:
        tag = MODULE_TAGS.get(module, module.title())

        for definition_name, definition in data.get("definitions", {}).items():
            existing = merged["definitions"].get(definition_name)
            if existing is None:
                merged["definitions"][definition_name] = definition
                continue
            if existing != definition:
                raise ValueError(
                    f"definition conflict for {definition_name} while merging {path}"
                )

        for route, methods in data.get("paths", {}).items():
            route_entry = merged["paths"].setdefault(route, {})
            for method, operation in methods.items():
                if method in route_entry:
                    raise ValueError(f"duplicate path {method.upper()} {route} from {path}")
                op = dict(operation)
                op["tags"] = [tag]
                op["operationId"] = prefixed_operation_id(module, op.get("operationId"))
                route_entry[method] = op

    merged["tags"].sort(key=lambda item: item["name"])
    merged["definitions"] = dict(sorted(merged["definitions"].items()))
    merged["paths"] = dict(sorted(merged["paths"].items()))
    return merged


def main() -> None:
    version = os.environ.get("DOC_VERSION", "").strip() or "unversioned"

    fragment_paths = sorted(
        path
        for path in ARTIFACTS_DIR.rglob("*.swagger.json")
        if path.name in {"query.swagger.json", "tx.swagger.json"}
    )
    if not fragment_paths:
        raise SystemExit("no OpenAPI fragments found under artifacts/docs")

    specs = [(module_from_path(path), path, load_json(path)) for path in fragment_paths]
    merged = merge_specs(specs, version)

    dump_json(MERGED_OPENAPI, merged)
    dump_json(STATIC_OPENAPI, merged)


if __name__ == "__main__":
    main()
