package minimax

import (
	"io"
	"strings"
	"testing"

	"mote/internal/provider"
)

func TestThinkTagParser_BasicThinking(t *testing.T) {
	events := make(chan provider.ChatEvent, 100)
	var p thinkTagParser

	// Simulate: <think>reasoning</think>answer
	p.Process("<think>", events)
	p.Process("reasoning", events)
	p.Process("</think>", events)
	p.Process("answer", events)

	// Collect events
	close(events)
	var thinkEvents, contentEvents []string
	for e := range events {
		if e.Type == provider.EventTypeThinking {
			thinkEvents = append(thinkEvents, e.Thinking)
		} else if e.Type == provider.EventTypeContent {
			contentEvents = append(contentEvents, e.Delta)
		}
	}

	if len(thinkEvents) != 1 || thinkEvents[0] != "reasoning" {
		t.Errorf("expected thinking='reasoning', got %v", thinkEvents)
	}
	if len(contentEvents) != 1 || contentEvents[0] != "answer" {
		t.Errorf("expected content='answer', got %v", contentEvents)
	}
}

func TestThinkTagParser_SplitTag(t *testing.T) {
	events := make(chan provider.ChatEvent, 100)
	var p thinkTagParser

	// Tag split across deltas: "<th" + "ink>" + "thought" + "</th" + "ink>"
	p.Process("<th", events)
	p.Process("ink>", events)
	p.Process("thought", events)
	p.Process("</th", events)
	p.Process("ink>", events)
	p.Process("done", events)

	close(events)
	var thinking, content string
	for e := range events {
		if e.Type == provider.EventTypeThinking {
			thinking += e.Thinking
		} else if e.Type == provider.EventTypeContent {
			content += e.Delta
		}
	}

	if thinking != "thought" {
		t.Errorf("expected thinking='thought', got %q", thinking)
	}
	if content != "done" {
		t.Errorf("expected content='done', got %q", content)
	}
}

func TestThinkTagParser_NoThinking(t *testing.T) {
	events := make(chan provider.ChatEvent, 100)
	var p thinkTagParser

	p.Process("hello world", events)

	close(events)
	var content string
	for e := range events {
		if e.Type == provider.EventTypeContent {
			content += e.Delta
		}
		if e.Type == provider.EventTypeThinking {
			t.Error("unexpected thinking event")
		}
	}

	if content != "hello world" {
		t.Errorf("expected 'hello world', got %q", content)
	}
}

func TestThinkTagParser_AngleBracketNotTag(t *testing.T) {
	events := make(chan provider.ChatEvent, 100)
	var p thinkTagParser

	// A '<' that isn't part of <think> should be emitted as content
	p.Process("x < y", events)

	close(events)
	var content string
	for e := range events {
		if e.Type == provider.EventTypeContent {
			content += e.Delta
		}
	}

	if content != "x < y" {
		t.Errorf("expected 'x < y', got %q", content)
	}
}

func TestThinkTagParser_InlineTag(t *testing.T) {
	events := make(chan provider.ChatEvent, 100)
	var p thinkTagParser

	// Full tag in single delta
	p.Process("<think>I'm thinking</think>Here's the answer", events)

	close(events)
	var thinking, content string
	for e := range events {
		if e.Type == provider.EventTypeThinking {
			thinking += e.Thinking
		} else if e.Type == provider.EventTypeContent {
			content += e.Delta
		}
	}

	if thinking != "I'm thinking" {
		t.Errorf("expected thinking='I'm thinking', got %q", thinking)
	}
	if content != "Here's the answer" {
		t.Errorf("expected content='Here's the answer', got %q", content)
	}
}

func TestProcessStream_ReasoningContent(t *testing.T) {
	// Simulate SSE stream with reasoning_content field (reasoning_split=True mode)
	sseData := `data: {"id":"1","object":"chat.completion.chunk","created":1234567890,"model":"MiniMax-M2.5","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"Let me think about this..."}}]}

data: {"id":"1","object":"chat.completion.chunk","created":1234567890,"model":"MiniMax-M2.5","choices":[{"index":0,"delta":{"reasoning_content":"The answer should be 42."}}]}

data: {"id":"1","object":"chat.completion.chunk","created":1234567890,"model":"MiniMax-M2.5","choices":[{"index":0,"delta":{"content":"The answer is 42."}}]}

data: {"id":"1","object":"chat.completion.chunk","created":1234567890,"model":"MiniMax-M2.5","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}

data: [DONE]
`

	reader := io.NopCloser(strings.NewReader(sseData))
	events := ProcessStream(reader)

	var thinking, content string
	var doneCount int
	for e := range events {
		switch e.Type {
		case provider.EventTypeThinking:
			thinking += e.Thinking
		case provider.EventTypeContent:
			content += e.Delta
		case provider.EventTypeDone:
			doneCount++
		}
	}

	if thinking != "Let me think about this...The answer should be 42." {
		t.Errorf("unexpected thinking: %q", thinking)
	}
	if content != "The answer is 42." {
		t.Errorf("unexpected content: %q", content)
	}
	if doneCount != 2 {
		t.Errorf("expected 2 done events (finish_reason + [DONE]), got %d", doneCount)
	}
}

func TestProcessStream_ReasoningDetails(t *testing.T) {
	// Simulate SSE stream with reasoning_details field
	sseData := `data: {"id":"1","object":"chat.completion.chunk","created":1234567890,"model":"MiniMax-M2.5","choices":[{"index":0,"delta":{"reasoning_details":[{"text":"Step 1: analyze"}]}}]}

data: {"id":"1","object":"chat.completion.chunk","created":1234567890,"model":"MiniMax-M2.5","choices":[{"index":0,"delta":{"reasoning_details":[{"text":"Step 2: compute"}]}}]}

data: {"id":"1","object":"chat.completion.chunk","created":1234567890,"model":"MiniMax-M2.5","choices":[{"index":0,"delta":{"content":"Result: 42"}}]}

data: {"id":"1","object":"chat.completion.chunk","created":1234567890,"model":"MiniMax-M2.5","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`

	reader := io.NopCloser(strings.NewReader(sseData))
	events := ProcessStream(reader)

	var thinking, content string
	for e := range events {
		switch e.Type {
		case provider.EventTypeThinking:
			thinking += e.Thinking
		case provider.EventTypeContent:
			content += e.Delta
		}
	}

	if thinking != "Step 1: analyzeStep 2: compute" {
		t.Errorf("unexpected thinking: %q", thinking)
	}
	if content != "Result: 42" {
		t.Errorf("unexpected content: %q", content)
	}
}
