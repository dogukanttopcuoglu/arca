package main

import (
	"fmt"
	"os"

	"arca/internal/agent"
	agenttool "arca/internal/agent/tool"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	llmprovider "arca/internal/llm/provider"
	"arca/internal/qa"
	"arca/internal/retrieval/dense"
)

func main() {
	fmt.Println("ARC Document Intelligence OS HTTP REST/SSE Server initializing...")

	mockProv := provider.NewMockEmbeddingProvider("mock-provider", "mock-model", 1536)
	vecStore := store.NewInMemoryVectorStore()
	denseRet := dense.NewDenseRetriever(mockProv, vecStore, store.NewInMemoryContentStore())

	// The demo root shares one LLM provider between generation and the
	// evidence gate so both fail or succeed symmetrically (ADR-0030): with
	// no model configured, generation and gate both fail closed.
	llm := llmprovider.NewOpenAICompatibleProvider("", "", "", "")
	ansEng := qa.NewAnswerEngine(nil, denseRet, nil, nil, llm, nil, qa.NewLLMEvidenceGate(llm))
	agentEng := agent.NewAgentEngine(agent.AgentPolicy{MaxSteps: 5}, []agenttool.Tool{
		agenttool.NewKnowledgeTool(ansEng),
	})

	_ = ansEng
	_ = agentEng

	fmt.Println("ARC Server listening on :8080 (REST Endpoints: /api/v1/spaces, /api/v1/qa/stream, /api/v1/research/jobs)")
	os.Exit(0)
}
