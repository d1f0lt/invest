package httpapi

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"invest/backend/services/gateway/internal/auth"
	"invest/backend/services/gateway/internal/static"
)

type brokerResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	FileFormats []string `json:"file_formats"`
	IconURL     string   `json:"icon_url,omitempty"`
	Color       string   `json:"color,omitempty"`
}

func (h *Handlers) handleListBrokers(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	resp, err := h.Upstream.Portfolio.ListBrokers(ctx, &emptypb.Empty{})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	out := make([]brokerResponse, 0, len(resp.GetBrokers()))
	for _, b := range resp.GetBrokers() {
		formats := b.GetFileFormats()
		if formats == nil {
			formats = []string{}
		}
		out = append(out, brokerResponse{
			ID:          b.GetId(),
			Name:        b.GetName(),
			FileFormats: formats,
			IconURL:     b.GetIconUrl(),
			Color:       b.GetColor(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}



func handleBrokerIcon(w http.ResponseWriter, r *http.Request) {
	name := path.Base(r.PathValue("file"))
	data, err := fs.ReadFile(static.FS, "brokers/"+name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, name, staticModTime, bytes.NewReader(data))
}



var staticModTime = time.Now()
