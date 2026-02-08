package channel

import (
	"context"
	"errors"
	"testing"

	"mote/pkg/channel"
)

// mockPlugin 是一个用于测试的模拟插件
type mockPlugin struct {
	id           channel.ChannelType
	name         string
	started      bool
	stopped      bool
	startErr     error
	stopErr      error
	handler      channel.MessageHandler
	capabilities channel.ChannelCapabilities
}

func newMockPlugin(id channel.ChannelType, name string) *mockPlugin {
	return &mockPlugin{
		id:   id,
		name: name,
		capabilities: channel.ChannelCapabilities{
			CanSendText: true,
			CanWatch:    true,
		},
	}
}

func (m *mockPlugin) ID() channel.ChannelType {
	return m.id
}

func (m *mockPlugin) Name() string {
	return m.name
}

func (m *mockPlugin) Capabilities() channel.ChannelCapabilities {
	return m.capabilities
}

func (m *mockPlugin) Start(ctx context.Context) error {
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	return nil
}

func (m *mockPlugin) Stop(ctx context.Context) error {
	if m.stopErr != nil {
		return m.stopErr
	}
	m.stopped = true
	return nil
}

func (m *mockPlugin) SendMessage(ctx context.Context, msg channel.OutboundMessage) error {
	return nil
}

func (m *mockPlugin) OnMessage(handler channel.MessageHandler) {
	m.handler = handler
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	p := newMockPlugin(channel.ChannelTypeIMessage, "iMessage")

	r.Register(p)

	if r.Count() != 1 {
		t.Errorf("Count() = %d, want 1", r.Count())
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()
	p := newMockPlugin(channel.ChannelTypeIMessage, "iMessage")
	r.Register(p)

	got, ok := r.Get(channel.ChannelTypeIMessage)
	if !ok {
		t.Error("Get() returned false for registered plugin")
	}
	if got.ID() != channel.ChannelTypeIMessage {
		t.Errorf("Get() returned plugin with ID %v, want %v", got.ID(), channel.ChannelTypeIMessage)
	}

	_, ok = r.Get(channel.ChannelTypeNotes)
	if ok {
		t.Error("Get() returned true for unregistered plugin")
	}
}

func TestRegistry_All(t *testing.T) {
	r := NewRegistry()
	p1 := newMockPlugin(channel.ChannelTypeIMessage, "iMessage")
	p2 := newMockPlugin(channel.ChannelTypeNotes, "Notes")

	r.Register(p1)
	r.Register(p2)

	all := r.All()
	if len(all) != 2 {
		t.Errorf("All() returned %d plugins, want 2", len(all))
	}
}

func TestRegistry_StartAll(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		r := NewRegistry()
		p1 := newMockPlugin(channel.ChannelTypeIMessage, "iMessage")
		p2 := newMockPlugin(channel.ChannelTypeNotes, "Notes")
		r.Register(p1)
		r.Register(p2)

		err := r.StartAll(context.Background())
		if err != nil {
			t.Errorf("StartAll() error = %v, want nil", err)
		}
		if !p1.started || !p2.started {
			t.Error("not all plugins were started")
		}
	})

	t.Run("error", func(t *testing.T) {
		r := NewRegistry()
		p := newMockPlugin(channel.ChannelTypeIMessage, "iMessage")
		p.startErr = errors.New("start failed")
		r.Register(p)

		err := r.StartAll(context.Background())
		if err == nil {
			t.Error("StartAll() error = nil, want error")
		}
	})
}

func TestRegistry_StopAll(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		r := NewRegistry()
		p1 := newMockPlugin(channel.ChannelTypeIMessage, "iMessage")
		p2 := newMockPlugin(channel.ChannelTypeNotes, "Notes")
		r.Register(p1)
		r.Register(p2)

		err := r.StopAll(context.Background())
		if err != nil {
			t.Errorf("StopAll() error = %v, want nil", err)
		}
		if !p1.stopped || !p2.stopped {
			t.Error("not all plugins were stopped")
		}
	})

	t.Run("error continues", func(t *testing.T) {
		r := NewRegistry()
		p1 := newMockPlugin(channel.ChannelTypeIMessage, "iMessage")
		p1.stopErr = errors.New("stop failed")
		p2 := newMockPlugin(channel.ChannelTypeNotes, "Notes")
		r.Register(p1)
		r.Register(p2)

		err := r.StopAll(context.Background())
		if err == nil {
			t.Error("StopAll() error = nil, want error")
		}
	})
}
