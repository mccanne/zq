package recruit

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/brimsec/zq/api"
	"github.com/brimsec/zq/cli"
	"github.com/brimsec/zq/zqe"
	"github.com/brimsec/zq/zql"
	"github.com/gorilla/mux"
)

type handlerFunc func(w http.ResponseWriter, r *http.Request)

type handler struct {
	*mux.Router
}

func (h *handler) Handle(path string, f handlerFunc) *mux.Route {
	return h.Router.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		f(w, r)
	})
}

func NewHandler(recruiter *Recruiter) http.Handler {
	h := handler{Router: mux.NewRouter(), core: core}
	h.Use(requestIDMiddleware())
	h.Use(accessLogMiddleware(logger))
	h.Use(panicCatchMiddleware(logger))
	h.Handle("/recruit", handleRecruit(recruiter)).Methods("POST")
	h.Handle("/register", handleRegister(recruiter)).Methods("POST")
	h.Handle("/deregister", handleDeregister(recruiter)).Methods("POST")
	h.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		json.NewEncoder(w).Encode(&api.VersionResponse{Version: cli.Version})
	})
	h.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	h.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, indexPage)
	})
	return h
}

const indexPage = `
<!DOCTYPE html>
<html>
  <title>ZQD recruiter</title>
  <body style="padding:10px">
    <h2>ZQD</h2>
    <p>A <a href="https://github.com/brimsec/zq/tree/master/cmd/zqd">zqd</a> daemon is listening on this host/port.</p>
    <p>If you're a <a href="https://www.brimsecurity.com/">Brim</a> user, connect to this host/port from the <a href="https://github.com/brimsec/brim">Brim application</a> in the graphical desktop interface in your operating system (not a web browser).</p>
    <p>If your goal is to perform command line operations against this zqd, use the <a href="https://github.com/brimsec/zq/tree/master/cmd/zapi">zapi</a> client.</p>
  </body>
</html>`

func respond(w http.ResponseWriter, r *http.Request, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		println("Error encoding reposnse body", err.Error())
	}
}

func respondError(w http.ResponseWriter, r *http.Request, err error) {
	status, ae := errorResponse(err)
	if status >= 500 {
		println("Error >= 500", err.Error())
	}
	respond(w, r, status, ae)
}

func request(w http.ResponseWriter, r *http.Request, apiobj interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(apiobj); err != nil {
		respondError(w, r, zqe.E(zqe.Invalid, err))
		return false
	}
	return true
}

func handleRecruit(w http.ResponseWriter, r *http.Request) {
	var req api.ASTRequest
	if !request(w, r, &req) {
		return
	}
	proc, err := zql.ParseProc(req.ZQL)
	if err != nil {
		respondError(w, r, zqe.ErrInvalid(err))
		return
	}
	respond(w, r, http.StatusOK, proc)
}
