"""
OpsAgent-AI Agent tools. Each tool calls the server-agent HTTP API to gather system diagnostics.
"""

import os
import requests

# Base URL of the server agent (default: http://localhost:8080)
SERVER_AGENT_URL = os.environ.get("SERVER_AGENT_URL", "http://localhost:8080")


def _get(endpoint: str) -> dict:
    """GET a server-agent endpoint and return JSON. Returns error dict on failure."""
    url = f"{SERVER_AGENT_URL.rstrip('/')}{endpoint}"
    try:
        r = requests.get(url, timeout=10)
        r.raise_for_status()
        return r.json()
    except requests.RequestException as e:
        return {"error": str(e), "endpoint": endpoint}
    except ValueError:
        return {"error": "Invalid JSON response", "endpoint": endpoint}


def check_cpu() -> dict:
    """
    Check CPU usage and top process on the server.
    Returns cpu_usage (int), top_process (str), and raw_sample.
    """
    return _get("/cpu")


def check_memory() -> dict:
    """
    Check memory usage (total, used, free, available in MB and usage_percent).
    """
    return _get("/memory")


def check_disk() -> dict:
    """
    Check disk usage per filesystem (size, used, avail, use_percent, mounted_on).
    """
    return _get("/disk")


def check_processes() -> dict:
    """
    Check top processes by CPU (user, pid, cpu%, mem%, command).
    """
    return _get("/processes")
