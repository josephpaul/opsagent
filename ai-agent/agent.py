"""
OpsAgent-AI Agent definition using Google ADK.
Supports Gemini (Google), Anthropic (Claude), and OpenAI models.
The agent uses tools to call the server-agent and diagnose Linux servers.
"""

import os
from typing import Optional

from google.adk.agents import Agent
from tools import check_cpu, check_memory, check_disk, check_processes

# Supported providers and default models
PROVIDERS = ("gemini", "anthropic", "openai")
DEFAULT_MODELS = {
    "gemini": "gemini-2.5-flash",
    "anthropic": "anthropic/claude-3-5-sonnet-20241022",
    "openai": "openai/gpt-4o",
}

AGENT_INSTRUCTION = """You are a DevOps diagnostic assistant. The user will ask about server health, slowness, or resource usage.

Use the available tools to gather data from the server agent:
- check_cpu: CPU usage and top process
- check_memory: Memory usage (total, used, free, available)
- check_disk: Disk usage per filesystem
- check_processes: Top processes by CPU

Based on the tool results, provide a clear diagnosis and details. If the user asks 'why is my server slow', call check_cpu, check_memory, and check_disk (and optionally check_processes), then summarize findings (e.g. high CPU, low memory, full disk) and name the main culprits (e.g. top process).

Keep the diagnosis concise and actionable. Format the response as a short summary (diagnosis) followed by details."""


def _get_model_config(provider: str, model: Optional[str] = None):
    """
    Return the ADK model argument for the given provider.
    Gemini uses a string; Anthropic and OpenAI use LiteLLM wrapper.
    """
    model_name = (model or "").strip() or DEFAULT_MODELS.get(provider, DEFAULT_MODELS["gemini"])
    provider_lower = provider.lower()

    if provider_lower == "gemini":
        return model_name or DEFAULT_MODELS["gemini"]

    if provider_lower in ("anthropic", "openai"):
        try:
            from google.adk.models.lite_llm import LiteLlm
        except ImportError as e:
            raise ImportError(
                "LiteLLM is required for Anthropic and OpenAI. Install with: pip install litellm"
            ) from e
        # LiteLLM format: "provider/model-name"
        if provider_lower == "openai" and "/" not in model_name:
            model_name = f"openai/{model_name}"
        elif provider_lower == "anthropic" and "/" not in model_name:
            model_name = f"anthropic/{model_name}"
        return LiteLlm(model=model_name)

    raise ValueError(f"Unsupported provider: {provider}. Choose from: {', '.join(PROVIDERS)}")


def get_agent(
    provider: Optional[str] = None,
    model: Optional[str] = None,
) -> Agent:
    """
    Create an ADK Agent for the given provider and optional model.
    provider: 'gemini' | 'anthropic' | 'openai' (default from AI_PROVIDER env, else 'gemini')
    model: optional model name override (e.g. 'gpt-4o', 'claude-3-5-sonnet-20241022', 'gemini-2.5-flash')
    """
    provider = (provider or os.environ.get("AI_PROVIDER", "gemini")).strip().lower()
    if provider not in PROVIDERS:
        raise ValueError(f"Unsupported provider: {provider}. Choose from: {', '.join(PROVIDERS)}")

    model_config = _get_model_config(provider, model)

    return Agent(
        name="opsagent_diagnostic_agent",
        model=model_config,
        description="Diagnoses Linux server issues by checking CPU, memory, disk, and processes.",
        instruction=AGENT_INSTRUCTION,
        tools=[check_cpu, check_memory, check_disk, check_processes],
    )


# Default agent instance (uses AI_PROVIDER and AI_MODEL from env)
def get_default_agent() -> Agent:
    """Return agent configured from environment (AI_PROVIDER, AI_MODEL)."""
    return get_agent(
        provider=os.environ.get("AI_PROVIDER"),
        model=os.environ.get("AI_MODEL"),
    )
