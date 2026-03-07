"""
OpsAgent-AI Agent HTTP API. Exposes POST /query for natural language server diagnostics.
Uses Google ADK for the agent and FastAPI for the HTTP server.
"""

import os

try:
    from dotenv import load_dotenv
    load_dotenv()
except ImportError:
    pass

from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel

# Default port for the AI agent
PORT = int(os.environ.get("PORT", "9000"))

app = FastAPI(
    title="OpsAgent-AI Agent API",
    description="Natural language server diagnostics via AI agent",
    version="1.0.0",
)
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


class QueryRequest(BaseModel):
    """Request body for POST /query."""

    question: str
    provider: str | None = None  # gemini | anthropic | openai (default from AI_PROVIDER env)
    model: str | None = None   # optional model override (e.g. gpt-4o, claude-3-5-sonnet, gemini-2.5-flash)


class QueryResponse(BaseModel):
    """Response body for POST /query."""

    diagnosis: str
    details: str


def extract_text_from_events(events: list) -> str:
    """
    Extract the final agent text response from ADK events.
    Events may have content with parts; we collect text from agent responses.
    """
    texts = []
    for event in events:
        content = getattr(event, "content", None)
        if content is None:
            continue
        parts = getattr(content, "parts", None) or []
        for part in parts:
            text = getattr(part, "text", None)
            if text:
                texts.append(text)
    return "\n\n".join(texts).strip() if texts else "No diagnosis generated."


@app.post("/query", response_model=QueryResponse)
async def query(req: QueryRequest) -> QueryResponse:
    """
    Accept a natural language question about the server, run the ADK agent
    (which may call server-agent tools), and return a human-readable diagnosis.
    Optional: provider (gemini|anthropic|openai), model (e.g. gpt-4o) to choose which LLM to use.
    """
    try:
        from google.adk.runners import InMemoryRunner
        from agent import get_agent
    except ImportError as e:
        raise HTTPException(
            status_code=503,
            detail=f"Agent not available: {e}. Install google-adk and set the appropriate API key (GOOGLE_API_KEY, ANTHROPIC_API_KEY, or OPENAI_API_KEY).",
        ) from e

    try:
        agent = get_agent(provider=req.provider, model=req.model)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e)) from e
    except ImportError as e:
        raise HTTPException(
            status_code=503,
            detail=f"Provider setup failed: {e}. For anthropic/openai install: pip install litellm",
        ) from e

    runner = InMemoryRunner(agent=agent)
    try:
        events = await runner.run_debug(
            req.question,
            quiet=True,
            verbose=False,
        )
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Agent run failed: {e}") from e

    full_text = extract_text_from_events(events)
    # Split into one-line diagnosis and rest as details
    lines = full_text.strip().split("\n")
    diagnosis = lines[0] if lines else "Diagnosis unavailable."
    details = "\n".join(lines[1:]).strip() if len(lines) > 1 else ""
    return QueryResponse(diagnosis=diagnosis, details=details)


@app.get("/health")
async def health() -> dict[str, str]:
    """Health check endpoint."""
    return {"status": "ok"}


@app.get("/providers")
async def providers() -> dict:
    """
    List supported AI providers and default models.
    Set AI_PROVIDER and optionally AI_MODEL in env, or pass provider/model in POST /query body.
    """
    from agent import PROVIDERS, DEFAULT_MODELS
    return {
        "providers": list(PROVIDERS),
        "default_models": DEFAULT_MODELS,
        "env": {
            "AI_PROVIDER": "gemini | anthropic | openai (default: gemini)",
            "AI_MODEL": "optional model override",
            "GOOGLE_API_KEY": "required for gemini",
            "ANTHROPIC_API_KEY": "required for anthropic",
            "OPENAI_API_KEY": "required for openai",
        },
    }


def main() -> None:
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=PORT)


if __name__ == "__main__":
    main()
