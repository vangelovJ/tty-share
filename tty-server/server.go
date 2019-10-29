package main

import (
	"html/template"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

const (
	errorInvalidSession = iota
	errorNotFound       = iota
	errorNotAllowed     = iota
)

var log = MainLogger

// SessionTemplateModel used for templating
type SessionTemplateModel struct {
	SessionID string
	Salt      string
	WSPath    string
}

// TTYServerConfig is used to configure the tty server before it is started
type TTYServerConfig struct {
	WebAddress   string
	FrontendPath string
	CommandName  string
	CommandArgs  string
}

// TTYServer represents the instance of a tty server
type TTYServer struct {
	httpServer           *http.Server
	config               TTYServerConfig
	activeSessions       map[string]*ptyMaster
	activeSessionsRWLock sync.RWMutex
}

// TTYServerError represents the instance of a tty server error
type TTYServerError struct {
	msg string
}

func (err *TTYServerError) Error() string {
	return err.msg
}

func (server *TTYServer) serveContent(w http.ResponseWriter, r *http.Request, name string) {
	// If a path to the frontend resources was passed, serve from there, otherwise, serve from the
	// builtin bundle
	if server.config.FrontendPath == "" {
		file, err := Asset(name)

		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		ctype := mime.TypeByExtension(filepath.Ext(name))
		if ctype == "" {
			ctype = http.DetectContentType(file)
		}
		w.Header().Set("Content-Type", ctype)
		w.Write(file)
	} else {
		filePath := server.config.FrontendPath + string(os.PathSeparator) + name
		_, err := os.Open(filePath)

		if err != nil {
			log.Errorf("Couldn't find resource: %s at %s", name, filePath)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		log.Debugf("Serving %s from %s", name, filePath)

		http.ServeFile(w, r, filePath)
	}
}

// NewTTYServer creates a new instance
func NewTTYServer(config TTYServerConfig) (server *TTYServer) {
	server = &TTYServer{
		config: config,
	}
	server.httpServer = &http.Server{
		Addr: config.WebAddress,
	}
	routesHandler := mux.NewRouter()

	routesHandler.PathPrefix("/static/").Handler(http.StripPrefix("/static/",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			server.serveContent(w, r, r.URL.Path)
		})))

	routesHandler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Default session
		http.Redirect(w, r, "/s/1", http.StatusMovedPermanently)
	})
	routesHandler.HandleFunc("/s/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		server.handleSession(w, r)
	})
	routesHandler.HandleFunc("/ws/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		server.handleWebsocket(w, r)
	})
	routesHandler.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.serveContent(w, r, "404.html")
	})

	server.activeSessions = make(map[string]*ptyMaster)
	server.httpServer.Handler = routesHandler
	return server
}

func getWSPath(sessionID string) string {
	return "/ws/" + sessionID
}

func (server *TTYServer) handleWebsocket(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionID"]
	defer log.Debug("Finished WS connection for ", sessionID)

	// Validate incoming request.
	if r.Method != "GET" {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// Upgrade to Websocket mode.
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Error("Cannot create the WS connection for session ", sessionID, ". Error: ", err.Error())
		return
	}

	session := server.getSession(sessionID)

	// No valid session with this ID, create a new one and start it
	if session == nil {
		session = server.createNewSession(sessionID)
		go func() {
			server.addSession(sessionID, session)
			session.Wait()
			log.Infof("Session %s stopped", sessionID)

			server.removeSession(session)
		}()
	}

	// TODO: attach the ptyMaster
	session.HandleReceiver(newWSConnection(conn))
}

func (server *TTYServer) handleSession(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionID"]

	log.Debugf("Handling web TTYReceiver session: %s", sessionID)

	session := server.getSession(sessionID)

	// No valid session with this ID, create a new one and start it
	if session == nil {
		session = server.createNewSession(sessionID)
		go func() {
			server.addSession(sessionID, session)
			session.Wait()
			log.Infof("Session %s stopped", sessionID)

			server.removeSession(session)
		}()
	}

	var t *template.Template
	var err error
	if server.config.FrontendPath == "" {
		templateDta, err := Asset("tty-receiver.in.html")

		if err != nil {
			panic("Cannot find the tty-receiver html template")
		}

		t = template.New("tty-receiver.html")
		_, err = t.Parse(string(templateDta))
	} else {
		t, err = template.ParseFiles(server.config.FrontendPath + string(os.PathSeparator) + "tty-receiver.in.html")
	}

	if err != nil {
		panic("Cannot parse the tty-receiver html template")
	}

	templateModel := SessionTemplateModel{
		SessionID: sessionID,
		Salt:      "salt&pepper",
		WSPath:    getWSPath(sessionID),
	}
	err = t.Execute(w, templateModel)

	if err != nil {
		panic("Cannot execute the tty-receiver html template")
	}
}

func (server *TTYServer) removeSession(session *ptyMaster) {
	server.activeSessionsRWLock.Lock()
	delete(server.activeSessions, session.GetSessionID())
	server.activeSessionsRWLock.Unlock()
}

func (server *TTYServer) addSession(sessionID string, session *ptyMaster) (err error) {
	server.activeSessionsRWLock.Lock()
	var ok bool
	if _, ok = server.activeSessions[sessionID]; ok {
		log.Warnf("Can not add session %s: already exists", sessionID)
		return &TTYServerError{msg: "Session exists"}
	}
	server.activeSessions[sessionID] = session
	server.activeSessionsRWLock.Unlock()
	return
}

func (server *TTYServer) createNewSession(sessionID string) (session *ptyMaster) {
	session = ptyMasterNew(sessionID)
	session.Start(server.config.CommandName, strings.Fields(server.config.CommandArgs))
	return
}

func (server *TTYServer) getSession(sessionID string) (session *ptyMaster) {
	// TODO: move this in a better place
	server.activeSessionsRWLock.RLock()
	session = server.activeSessions[sessionID]
	server.activeSessionsRWLock.RUnlock()
	return
}

// Listen starts listening on connections
func (server *TTYServer) Listen() (err error) {
	err = server.httpServer.ListenAndServe()
	log.Debug("Server finished")
	return
}

// Stop closes down the server
func (server *TTYServer) Stop() (err error) {
	log.Debug("Stopping the server")
	err = server.httpServer.Close()
	return
}
