package httpapi

import (
	"net/http"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"invest/backend/services/gateway/internal/auth"
	notifierpb "invest/backend/services/gateway/internal/notifierpb"
)

type createAlertRequest struct {
	SecID         string   `json:"secid"`
	Board         string   `json:"board,omitempty"`
	TargetPrice   *float64 `json:"target_price,omitempty"`
	ChangePercent *float64 `json:"change_percent,omitempty"`
}

type updateAlertRequest struct {
	SecID         string   `json:"secid,omitempty"`
	Board         string   `json:"board,omitempty"`
	TargetPrice   *float64 `json:"target_price,omitempty"`
	ChangePercent *float64 `json:"change_percent,omitempty"`
}

type alertResponse struct {
	ID             string     `json:"id"`
	SecID          string     `json:"secid"`
	Board          string     `json:"board"`
	ShortName      string     `json:"short_name"`
	Direction      string     `json:"direction"`
	BasePrice      float64    `json:"base_price"`
	TargetPrice    float64    `json:"target_price"`
	ChangePercent  float64    `json:"change_percent"`
	InputPercent   *float64   `json:"input_percent"`
	CurrentPrice   *float64   `json:"current_price"`
	PriceInPercent bool       `json:"price_in_percent"`
	Currency       string     `json:"currency"`
	Status         string     `json:"status"`
	TriggeredAt    *time.Time `json:"triggered_at"`
	TriggeredPrice *float64   `json:"triggered_price"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

var alertStatusNames = map[notifierpb.AlertStatus]string{
	notifierpb.AlertStatus_ALERT_STATUS_ACTIVE:    "active",
	notifierpb.AlertStatus_ALERT_STATUS_TRIGGERED: "triggered",
}

var directionNames = map[notifierpb.Direction]string{
	notifierpb.Direction_DIRECTION_ABOVE: "above",
	notifierpb.Direction_DIRECTION_BELOW: "below",
}

func toAlertResponse(a *notifierpb.Alert) alertResponse {
	out := alertResponse{
		ID: a.GetId(), SecID: a.GetSecid(), Board: a.GetBoard(), ShortName: a.GetShortName(),
		Direction: directionNames[a.GetDirection()],
		BasePrice: a.GetBasePrice(), TargetPrice: a.GetTargetPrice(), ChangePercent: a.GetChangePercent(),
		CurrentPrice: a.CurrentPrice, PriceInPercent: a.GetPriceInPercent(), Currency: a.GetCurrency(),
		Status: alertStatusNames[a.GetStatus()], TriggeredPrice: a.TriggeredPrice, InputPercent: a.InputPercent,
		CreatedAt: a.GetCreatedAt().AsTime(), UpdatedAt: a.GetUpdatedAt().AsTime(),
	}
	if a.TriggeredAt != nil {
		t := a.GetTriggeredAt().AsTime()
		out.TriggeredAt = &t
	}
	return out
}

type listAlertsResponse struct {
	Alerts []alertResponse `json:"alerts"`
}

type telegramLinkTokenResponse struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type telegramLinkResponse struct {
	Linked     bool       `json:"linked"`
	Username   string     `json:"username,omitempty"`
	LinkedAt   *time.Time `json:"linked_at,omitempty"`
	BotEnabled bool       `json:"bot_enabled"`
}

func toCreateAlertPB(req createAlertRequest) (*notifierpb.CreateAlertRequest, string) {
	if req.SecID == "" {
		return nil, "secid is required"
	}
	if (req.TargetPrice == nil) == (req.ChangePercent == nil) {
		return nil, "exactly one of target_price or change_percent is required"
	}
	out := &notifierpb.CreateAlertRequest{Secid: req.SecID, Board: req.Board}
	if req.TargetPrice != nil {
		out.Target = &notifierpb.CreateAlertRequest_TargetPrice{TargetPrice: *req.TargetPrice}
	} else {
		out.Target = &notifierpb.CreateAlertRequest_ChangePercent{ChangePercent: *req.ChangePercent}
	}
	return out, ""
}

func toUpdateAlertPB(id string, req updateAlertRequest) (*notifierpb.UpdateAlertRequest, string) {
	if (req.TargetPrice == nil) == (req.ChangePercent == nil) {
		return nil, "exactly one of target_price or change_percent is required"
	}
	out := &notifierpb.UpdateAlertRequest{Id: id, Secid: req.SecID, Board: req.Board}
	if req.TargetPrice != nil {
		out.Target = &notifierpb.UpdateAlertRequest_TargetPrice{TargetPrice: *req.TargetPrice}
	} else {
		out.Target = &notifierpb.UpdateAlertRequest_ChangePercent{ChangePercent: *req.ChangePercent}
	}
	return out, ""
}

func (h *Handlers) handleUpdateAlert(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	var req updateAlertRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	pbReq, problem := toUpdateAlertPB(r.PathValue("id"), req)
	if problem != "" {
		writeError(w, http.StatusBadRequest, problem)
		return
	}

	a, err := h.Upstream.Notifier.UpdateAlert(ctx, pbReq)
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, toAlertResponse(a))
}

func (h *Handlers) handleCreateAlert(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	var req createAlertRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	pbReq, problem := toCreateAlertPB(req)
	if problem != "" {
		writeError(w, http.StatusBadRequest, problem)
		return
	}

	a, err := h.Upstream.Notifier.CreateAlert(ctx, pbReq)
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAlertResponse(a))
}

func (h *Handlers) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	resp, err := h.Upstream.Notifier.ListAlerts(ctx, &emptypb.Empty{})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	out := listAlertsResponse{Alerts: make([]alertResponse, 0, len(resp.GetAlerts()))}
	for _, a := range resp.GetAlerts() {
		out.Alerts = append(out.Alerts, toAlertResponse(a))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) handleDeleteAlert(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	if _, err := h.Upstream.Notifier.DeleteAlert(ctx, &notifierpb.DeleteAlertRequest{Id: r.PathValue("id")}); err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleCreateTelegramLink(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	resp, err := h.Upstream.Notifier.CreateTelegramLink(ctx, &emptypb.Empty{})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, telegramLinkTokenResponse{URL: resp.GetUrl(), ExpiresAt: resp.GetExpiresAt().AsTime()})
}

func (h *Handlers) handleGetTelegramLink(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	resp, err := h.Upstream.Notifier.GetTelegramLink(ctx, &emptypb.Empty{})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, toTelegramLinkResponse(resp))
}

func toTelegramLinkResponse(l *notifierpb.TelegramLink) telegramLinkResponse {
	out := telegramLinkResponse{Linked: l.GetLinked(), Username: l.GetUsername(), BotEnabled: l.GetBotEnabled()}
	if l.LinkedAt != nil {
		t := l.GetLinkedAt().AsTime()
		out.LinkedAt = &t
	}
	return out
}

func (h *Handlers) handleDeleteTelegramLink(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	if _, err := h.Upstream.Notifier.DeleteTelegramLink(ctx, &emptypb.Empty{}); err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
