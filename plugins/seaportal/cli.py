#!/usr/bin/env python3
"""
SeaPortal SMCP Plugin

Exposes SeaPortal's CLI as MCP tools for use with SMCP (sanctumos/smcp).
Operations: extract (markdown/JSON), snapshot (accessibility tree).

Copyright (c) 2026 pinchtab.
"""

import argparse
import json
import subprocess
import sys
from typing import Any, Dict, List, Optional

PLUGIN_VERSION = "0.1.0"


def _error_response(error: str, error_type: str) -> Dict[str, Any]:
    return {
        "status": "error",
        "error": error,
        "error_type": error_type,
    }


def _canonical_option_name(action: argparse.Action) -> str:
    for opt in action.option_strings:
        if opt.startswith("--"):
            return opt[2:].replace("_", "-")
    return action.dest.replace("_", "-")


def _arg_type_name(action: argparse.Action) -> str:
    if isinstance(action, argparse._StoreTrueAction):
        return "boolean"
    if getattr(action, "type", None) is int:
        return "integer"
    if getattr(action, "type", None) is float:
        return "number"
    return "string"


def _describe_action(action: argparse.Action) -> Optional[Dict[str, Any]]:
    if action.dest in ("help", "command") or getattr(action, "help", None) == argparse.SUPPRESS:
        return None
    default = None if action.default is argparse.SUPPRESS else action.default
    return {
        "name": _canonical_option_name(action),
        "type": _arg_type_name(action),
        "description": (action.help or "").strip(),
        "required": bool(getattr(action, "required", False)),
        "default": default,
    }


def _get_subparsers_action(parser: argparse.ArgumentParser) -> Optional[argparse._SubParsersAction]:
    for a in parser._actions:
        if isinstance(a, argparse._SubParsersAction):
            return a
    return None


def _run_seaportal(args: List[str]) -> Dict[str, Any]:
    """Run seaportal CLI and capture output."""
    try:
        result = subprocess.run(
            ["seaportal"] + args,
            capture_output=True,
            text=True,
            timeout=60,
        )
        if result.returncode != 0:
            return _error_response(result.stderr or "Command failed", "cli_error")
        
        # Try to parse as JSON
        output = result.stdout.strip()
        try:
            data = json.loads(output)
            return {"status": "success", "data": data}
        except json.JSONDecodeError:
            # Return as text
            return {"status": "success", "data": {"text": output}}
    except FileNotFoundError:
        return _error_response("seaportal not found in PATH", "not_found")
    except subprocess.TimeoutExpired:
        return _error_response("Command timed out", "timeout")
    except Exception as e:
        return _error_response(str(e), "unknown_error")


def cmd_extract(args: Dict[str, Any]) -> Dict[str, Any]:
    """Extract content from URL as markdown or JSON."""
    url = args.get("url")
    if not url:
        return _error_response("url is required", "validation_error")
    
    cli_args = []
    if args.get("json"):
        cli_args.append("--json")
    if args.get("fast"):
        cli_args.append("--fast")
    if args.get("no-dedupe"):
        cli_args.append("--no-dedupe")
    cli_args.append(url)
    
    return _run_seaportal(cli_args)


def cmd_snapshot(args: Dict[str, Any]) -> Dict[str, Any]:
    """Get accessibility tree snapshot from URL."""
    url = args.get("url")
    if not url:
        return _error_response("url is required", "validation_error")
    
    cli_args = ["--snapshot"]
    if args.get("filter") == "interactive":
        cli_args.extend(["--filter", "interactive"])
    if args.get("format"):
        cli_args.extend(["--format", args["format"]])
    if args.get("max-tokens"):
        cli_args.extend(["--max-tokens", str(args["max-tokens"])])
    cli_args.append(url)
    
    return _run_seaportal(cli_args)


def cmd_version(args: Dict[str, Any]) -> Dict[str, Any]:
    """Get seaportal version."""
    return _run_seaportal(["--version"])


def get_plugin_description(parser: argparse.ArgumentParser) -> Dict[str, Any]:
    commands: List[Dict[str, Any]] = []
    sub = _get_subparsers_action(parser)
    if sub:
        for cmd_name, cmd_parser in sub.choices.items():
            params = []
            for a in cmd_parser._actions:
                d = _describe_action(a)
                if d:
                    params.append(d)
            commands.append({
                "name": cmd_name,
                "description": (cmd_parser.description or "").strip(),
                "parameters": params,
            })
    return {
        "plugin": {
            "name": "seaportal",
            "version": PLUGIN_VERSION,
            "description": "Web content extraction for AI agents. Extract markdown/JSON from URLs, get accessibility tree snapshots with element refs.",
        },
        "commands": commands,
    }


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="SeaPortal SMCP plugin — web content extraction",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--describe", action="store_true", help="Output plugin description JSON")
    sub = parser.add_subparsers(dest="command", help="Commands")

    # extract
    p_extract = sub.add_parser("extract", description="Extract content from URL as markdown or JSON")
    p_extract.add_argument("--url", required=True, help="URL to extract content from")
    p_extract.add_argument("--json", action="store_true", help="Output as JSON with metadata")
    p_extract.add_argument("--fast", action="store_true", help="Bail early if browser needed")
    p_extract.add_argument("--no-dedupe", dest="no_dedupe", action="store_true", help="Disable deduplication")

    # snapshot
    p_snap = sub.add_parser("snapshot", description="Get accessibility tree snapshot from URL")
    p_snap.add_argument("--url", required=True, help="URL to snapshot")
    p_snap.add_argument("--filter", choices=["interactive"], help="Filter to interactive elements only")
    p_snap.add_argument("--format", choices=["json", "compact"], help="Output format")
    p_snap.add_argument("--max-tokens", dest="max_tokens", type=int, help="Approximate token limit")

    # version
    sub.add_parser("version", description="Get seaportal version")

    return parser


def main() -> None:
    parser = build_parser()
    try:
        args = parser.parse_args()
    except SystemExit as e:
        if e.code == 0:
            raise
        err = _error_response("Invalid arguments", "argument_error")
        print(json.dumps(err, indent=2))
        sys.exit(e.code if isinstance(e.code, int) else 2)

    if args.describe:
        print(json.dumps(get_plugin_description(parser), indent=2))
        sys.exit(0)
    
    if not args.command:
        parser.print_help()
        sys.exit(1)

    args_dict: Dict[str, Any] = {}
    for k, v in vars(args).items():
        if k in ("command", "describe"):
            continue
        if v is None:
            continue
        key = k.replace("_", "-")
        args_dict[key] = v

    commands = {
        "extract": cmd_extract,
        "snapshot": cmd_snapshot,
        "version": cmd_version,
    }
    
    fn = commands.get(args.command)
    if not fn:
        result = _error_response(f"Unknown command: {args.command}", "argument_error")
    else:
        result = fn(args_dict)
    
    print(json.dumps(result, indent=2))
    sys.exit(0 if result.get("status") == "success" else 1)


if __name__ == "__main__":
    main()
