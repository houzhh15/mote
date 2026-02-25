package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/viper"

	internalChannel "mote/internal/channel"
	"mote/internal/gateway/handlers"
	"mote/pkg/channel"
)

// ChannelStatus 渠道状态响应
type ChannelStatus struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"` // running, stopped, error
	Error   string `json:"error,omitempty"`
}

// IMessageChannelConfigResponse iMessage 配置响应
type IMessageChannelConfigResponse struct {
	Enabled   bool              `json:"enabled"`
	Model     string            `json:"model,omitempty"`
	SelfID    string            `json:"selfId,omitempty"`
	Trigger   TriggerConfigResp `json:"trigger"`
	Reply     ReplyConfigResp   `json:"reply"`
	AllowFrom []string          `json:"allowFrom"`
}

type TriggerConfigResp struct {
	Prefix        string `json:"prefix"`
	CaseSensitive bool   `json:"caseSensitive"`
	SelfTrigger   bool   `json:"selfTrigger"`
}

type ReplyConfigResp struct {
	Prefix    string `json:"prefix"`
	Separator string `json:"separator"`
}

// IMessageChannelConfigRequest iMessage 配置请求
type IMessageChannelConfigRequest struct {
	Enabled   bool             `json:"enabled"`
	Model     string           `json:"model,omitempty"`
	SelfID    string           `json:"selfId,omitempty"`
	Trigger   TriggerConfigReq `json:"trigger"`
	Reply     ReplyConfigReq   `json:"reply"`
	AllowFrom []string         `json:"allowFrom"`
}

type TriggerConfigReq struct {
	Prefix        string `json:"prefix"`
	CaseSensitive bool   `json:"caseSensitive"`
	SelfTrigger   bool   `json:"selfTrigger"`
}

type ReplyConfigReq struct {
	Prefix    string `json:"prefix"`
	Separator string `json:"separator"`
}

// AppleNotesChannelConfigResponse Apple Notes 配置响应
type AppleNotesChannelConfigResponse struct {
	Enabled       bool              `json:"enabled"`
	Model         string            `json:"model,omitempty"`
	Trigger       TriggerConfigResp `json:"trigger"`
	Reply         ReplyConfigResp   `json:"reply"`
	WatchFolder   string            `json:"watchFolder"`
	ArchiveFolder string            `json:"archiveFolder"`
	PollInterval  string            `json:"pollInterval"`
}

// AppleNotesChannelConfigRequest Apple Notes 配置请求
type AppleNotesChannelConfigRequest struct {
	Enabled       bool             `json:"enabled"`
	Model         string           `json:"model,omitempty"`
	Trigger       TriggerConfigReq `json:"trigger"`
	Reply         ReplyConfigReq   `json:"reply"`
	WatchFolder   string           `json:"watchFolder"`
	ArchiveFolder string           `json:"archiveFolder"`
	PollInterval  string           `json:"pollInterval"`
}

// AppleRemindersChannelConfigResponse Apple Reminders 配置响应
type AppleRemindersChannelConfigResponse struct {
	Enabled      bool              `json:"enabled"`
	Model        string            `json:"model,omitempty"`
	Trigger      TriggerConfigResp `json:"trigger"`
	Reply        ReplyConfigResp   `json:"reply"`
	WatchList    string            `json:"watchList"`
	PollInterval string            `json:"pollInterval"`
}

// AppleRemindersChannelConfigRequest Apple Reminders 配置请求
type AppleRemindersChannelConfigRequest struct {
	Enabled      bool             `json:"enabled"`
	Model        string           `json:"model,omitempty"`
	Trigger      TriggerConfigReq `json:"trigger"`
	Reply        ReplyConfigReq   `json:"reply"`
	WatchList    string           `json:"watchList"`
	PollInterval string           `json:"pollInterval"`
}

// SetChannelRegistry 设置 channel registry 依赖
func (r *Router) SetChannelRegistry(registry *internalChannel.Registry) {
	r.channelRegistry = registry
}

// supportedChannels 定义所有可配置的渠道类型
var supportedChannels = []struct {
	Type string
	Name string
}{
	{Type: string(channel.ChannelTypeIMessage), Name: "iMessage"},
	{Type: string(channel.ChannelTypeNotes), Name: "Apple Notes"},
	{Type: string(channel.ChannelTypeReminders), Name: "Apple Reminders"},
}

// HandleListChannels 返回所有渠道状态列表
func (r *Router) HandleListChannels(w http.ResponseWriter, req *http.Request) {
	statuses := make([]ChannelStatus, 0, len(supportedChannels))

	// 构建已启动渠道的映射（用于检查运行状态）
	// 优先从 runner 获取最新的 registry（StartChannel 可能会创建新渠道）
	runningChannels := make(map[string]bool)
	registry := r.channelRegistry
	if r.runner != nil {
		if runnerRegistry := r.runner.ChannelRegistry(); runnerRegistry != nil {
			registry = runnerRegistry
		}
	}
	if registry != nil {
		for _, p := range registry.All() {
			runningChannels[string(p.ID())] = true
		}
	}

	// 遍历所有支持的渠道类型
	for _, ch := range supportedChannels {
		enabled := viper.GetBool("channels." + ch.Type + ".enabled")
		status := ChannelStatus{
			Type:    ch.Type,
			Name:    ch.Name,
			Enabled: enabled,
			Status:  "stopped",
		}

		// 检查是否正在运行
		if runningChannels[ch.Type] && enabled {
			status.Status = "running"
		}

		statuses = append(statuses, status)
	}

	handlers.SendJSON(w, http.StatusOK, statuses)
}

// HandleGetChannelConfig 获取指定渠道配置
func (r *Router) HandleGetChannelConfig(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	channelType := vars["type"]

	switch channelType {
	case string(channel.ChannelTypeIMessage):
		r.getIMessageConfig(w)
	case string(channel.ChannelTypeNotes):
		r.getAppleNotesConfig(w)
	case string(channel.ChannelTypeReminders):
		r.getAppleRemindersConfig(w)
	default:
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "channel not found")
	}
}

func (r *Router) getIMessageConfig(w http.ResponseWriter) {
	config := IMessageChannelConfigResponse{
		Enabled: viper.GetBool("channels.imessage.enabled"),
		Model:   viper.GetString("channels.imessage.model"),
		SelfID:  viper.GetString("channels.imessage.self_id"),
		Trigger: TriggerConfigResp{
			Prefix:        viper.GetString("channels.imessage.trigger.prefix"),
			CaseSensitive: viper.GetBool("channels.imessage.trigger.case_sensitive"),
			SelfTrigger:   viper.GetBool("channels.imessage.trigger.self_trigger"),
		},
		Reply: ReplyConfigResp{
			Prefix:    viper.GetString("channels.imessage.reply.prefix"),
			Separator: viper.GetString("channels.imessage.reply.separator"),
		},
		AllowFrom: viper.GetStringSlice("channels.imessage.allow_from"),
	}

	// 设置默认值
	if config.Trigger.Prefix == "" {
		config.Trigger.Prefix = "@mote"
	}
	if config.Reply.Prefix == "" {
		config.Reply.Prefix = "[Mote]"
	}
	if config.Reply.Separator == "" {
		config.Reply.Separator = "\n"
	}
	if config.AllowFrom == nil {
		config.AllowFrom = []string{}
	}

	handlers.SendJSON(w, http.StatusOK, config)
}

func (r *Router) getAppleNotesConfig(w http.ResponseWriter) {
	config := AppleNotesChannelConfigResponse{
		Enabled: viper.GetBool("channels.apple_notes.enabled"),
		Model:   viper.GetString("channels.apple_notes.model"),
		Trigger: TriggerConfigResp{
			Prefix:        viper.GetString("channels.apple_notes.trigger.prefix"),
			CaseSensitive: viper.GetBool("channels.apple_notes.trigger.case_sensitive"),
		},
		Reply: ReplyConfigResp{
			Prefix:    viper.GetString("channels.apple_notes.reply.prefix"),
			Separator: viper.GetString("channels.apple_notes.reply.separator"),
		},
		WatchFolder:   viper.GetString("channels.apple_notes.watch_folder"),
		ArchiveFolder: viper.GetString("channels.apple_notes.archive_folder"),
		PollInterval:  viper.GetString("channels.apple_notes.poll_interval"),
	}

	// 设置默认值
	if config.Trigger.Prefix == "" {
		config.Trigger.Prefix = "@mote:"
	}
	if config.Reply.Prefix == "" {
		config.Reply.Prefix = "[Mote 回复]"
	}
	if config.WatchFolder == "" {
		config.WatchFolder = "Mote Inbox"
	}
	if config.ArchiveFolder == "" {
		config.ArchiveFolder = "Mote Archive"
	}
	if config.PollInterval == "" {
		config.PollInterval = "5s"
	}

	handlers.SendJSON(w, http.StatusOK, config)
}

func (r *Router) getAppleRemindersConfig(w http.ResponseWriter) {
	config := AppleRemindersChannelConfigResponse{
		Enabled: viper.GetBool("channels.apple_reminders.enabled"),
		Model:   viper.GetString("channels.apple_reminders.model"),
		Trigger: TriggerConfigResp{
			Prefix:        viper.GetString("channels.apple_reminders.trigger.prefix"),
			CaseSensitive: viper.GetBool("channels.apple_reminders.trigger.case_sensitive"),
		},
		Reply: ReplyConfigResp{
			Prefix:    viper.GetString("channels.apple_reminders.reply.prefix"),
			Separator: viper.GetString("channels.apple_reminders.reply.separator"),
		},
		WatchList:    viper.GetString("channels.apple_reminders.watch_list"),
		PollInterval: viper.GetString("channels.apple_reminders.poll_interval"),
	}

	// 设置默认值
	if config.Trigger.Prefix == "" {
		config.Trigger.Prefix = "@mote:"
	}
	if config.Reply.Prefix == "" {
		config.Reply.Prefix = "[Mote]"
	}
	if config.WatchList == "" {
		config.WatchList = "Mote"
	}
	if config.PollInterval == "" {
		config.PollInterval = "5s"
	}

	handlers.SendJSON(w, http.StatusOK, config)
}

// HandleUpdateChannelConfig 更新指定渠道配置
func (r *Router) HandleUpdateChannelConfig(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	channelType := vars["type"]

	switch channelType {
	case string(channel.ChannelTypeIMessage):
		r.updateIMessageConfig(w, req)
	case string(channel.ChannelTypeNotes):
		r.updateAppleNotesConfig(w, req)
	case string(channel.ChannelTypeReminders):
		r.updateAppleRemindersConfig(w, req)
	default:
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "channel not found")
	}
}

func (r *Router) updateIMessageConfig(w http.ResponseWriter, req *http.Request) {
	var body IMessageChannelConfigRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "invalid request body: "+err.Error())
		return
	}

	viper.Set("channels.imessage.enabled", body.Enabled)
	viper.Set("channels.imessage.model", body.Model)
	viper.Set("channels.imessage.self_id", body.SelfID)
	viper.Set("channels.imessage.trigger.prefix", body.Trigger.Prefix)
	viper.Set("channels.imessage.trigger.case_sensitive", body.Trigger.CaseSensitive)
	viper.Set("channels.imessage.trigger.self_trigger", body.Trigger.SelfTrigger)
	viper.Set("channels.imessage.reply.prefix", body.Reply.Prefix)
	viper.Set("channels.imessage.reply.separator", body.Reply.Separator)
	viper.Set("channels.imessage.allow_from", body.AllowFrom)

	if err := viper.WriteConfig(); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "failed to save config: "+err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (r *Router) updateAppleNotesConfig(w http.ResponseWriter, req *http.Request) {
	var body AppleNotesChannelConfigRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "invalid request body: "+err.Error())
		return
	}

	viper.Set("channels.apple_notes.enabled", body.Enabled)
	viper.Set("channels.apple_notes.model", body.Model)
	viper.Set("channels.apple_notes.trigger.prefix", body.Trigger.Prefix)
	viper.Set("channels.apple_notes.trigger.case_sensitive", body.Trigger.CaseSensitive)
	viper.Set("channels.apple_notes.reply.prefix", body.Reply.Prefix)
	viper.Set("channels.apple_notes.reply.separator", body.Reply.Separator)
	viper.Set("channels.apple_notes.watch_folder", body.WatchFolder)
	viper.Set("channels.apple_notes.archive_folder", body.ArchiveFolder)
	viper.Set("channels.apple_notes.poll_interval", body.PollInterval)

	if err := viper.WriteConfig(); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "failed to save config: "+err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (r *Router) updateAppleRemindersConfig(w http.ResponseWriter, req *http.Request) {
	var body AppleRemindersChannelConfigRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "invalid request body: "+err.Error())
		return
	}

	viper.Set("channels.apple_reminders.enabled", body.Enabled)
	viper.Set("channels.apple_reminders.model", body.Model)
	viper.Set("channels.apple_reminders.trigger.prefix", body.Trigger.Prefix)
	viper.Set("channels.apple_reminders.trigger.case_sensitive", body.Trigger.CaseSensitive)
	viper.Set("channels.apple_reminders.reply.prefix", body.Reply.Prefix)
	viper.Set("channels.apple_reminders.reply.separator", body.Reply.Separator)
	viper.Set("channels.apple_reminders.watch_list", body.WatchList)
	viper.Set("channels.apple_reminders.poll_interval", body.PollInterval)

	if err := viper.WriteConfig(); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "failed to save config: "+err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleStartChannel 启动指定渠道
func (r *Router) HandleStartChannel(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	channelType := vars["type"]

	if r.runner == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "runner not available")
		return
	}

	// 使用 Background context，因为渠道需要长期运行，不能随请求结束而取消
	if err := r.runner.StartChannel(context.Background(), channel.ChannelType(channelType)); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "failed to start channel: "+err.Error())
		return
	}

	// 更新 enabled 配置
	viper.Set("channels."+channelType+".enabled", true)
	_ = viper.WriteConfig()

	handlers.SendJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleStopChannel 停止指定渠道
func (r *Router) HandleStopChannel(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	channelType := vars["type"]

	if r.runner == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "runner not available")
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()

	if err := r.runner.StopChannel(ctx, channel.ChannelType(channelType)); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "failed to stop channel: "+err.Error())
		return
	}

	// 更新 enabled 配置
	viper.Set("channels."+channelType+".enabled", false)
	_ = viper.WriteConfig()

	handlers.SendJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
