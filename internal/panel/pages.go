package panel

import (
	"bytes"
	"context"
	"embed"
	"html/template"
	"net/http"

	"github.com/bradfordly/bradfordly-games/internal/world"
)

//go:embed templates/*.html
var pageFS embed.FS

var pages = template.Must(template.ParseFS(pageFS, "templates/*.html"))

// States is gateway-owned runtime the panel reads.
type States interface {
	Runtime(ctx context.Context, id string) (world.Runtime, bool)
}

// MemoryStates is a test map of gateway state.
type MemoryStates struct {
	ByID map[string]world.Runtime
}

// Runtime returns a stored snapshot.
func (m MemoryStates) Runtime(_ context.Context, id string) (world.Runtime, bool) {
	rt, ok := m.ByID[id]
	return rt, ok
}

func (s *Server) view(ctx context.Context, item world.World) world.View {
	v := world.View{Record: item.Record()}
	if s.cfg.States != nil {
		if rt, ok := s.cfg.States.Runtime(ctx, item.ID()); ok {
			v.Runtime = rt
			return v
		}
	}
	v.State = "asleep"
	return v
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	items, err := s.cfg.Store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views := make([]world.View, 0, len(items))
	for _, item := range items {
		views = append(views, s.view(r.Context(), item))
	}
	render(w, "list.html", map[string]any{"Worlds": views})
}

func (s *Server) handleWorldDetail(w http.ResponseWriter, r *http.Request) {
	item, err := s.cfg.Store.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	render(w, "detail.html", s.view(r.Context(), item))
}

func render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := pages.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}
