// Package functions holds the starter handlers for scale-to-zero functions.
package functions

import "sort"

// DefaultPort is the port every starter handler listens on unless PORT says otherwise.
const DefaultPort = 8080

// File is one file of a starter handler.
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Runtime is a language Railpack builds without a Dockerfile.
type Runtime struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Files []File `json:"files"`
}

var runtimes = []Runtime{
	{
		ID:   "node",
		Name: "Node.js",
		Files: []File{
			{Path: "package.json", Content: `{
  "name": "handler",
  "version": "1.0.0",
  "scripts": { "start": "node index.js" }
}
`},
			{Path: "index.js", Content: `const http = require("http");

http.createServer((req, res) => {
  res.setHeader("content-type", "application/json");
  res.end(JSON.stringify({ message: "hello from a function", path: req.url }));
}).listen(process.env.PORT || 8080);
`},
		},
	},
	{
		ID:   "python",
		Name: "Python",
		Files: []File{
			{Path: "requirements.txt", Content: "\n"},
			{Path: "main.py", Content: `import json, os
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        body = json.dumps({"message": "hello from a function", "path": self.path}).encode()
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.end_headers()
        self.wfile.write(body)


HTTPServer(("", int(os.environ.get("PORT", "8080"))), Handler).serve_forever()
`},
		},
	},
	{
		ID:   "go",
		Name: "Go",
		Files: []File{
			{Path: "go.mod", Content: "module handler\n\ngo 1.22\n"},
			{Path: "main.go", Content: `package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		fmt.Fprintf(w, "{\"message\":\"hello from a function\",\"path\":%q}", r.URL.Path)
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	_ = http.ListenAndServe(":"+port, nil)
}
`},
		},
	},
}

// Runtimes lists the starter runtimes in a stable order.
func Runtimes() []Runtime {
	out := make([]Runtime, len(runtimes))
	copy(out, runtimes)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Lookup returns the runtime with the given id.
func Lookup(id string) (Runtime, bool) {
	for _, r := range runtimes {
		if r.ID == id {
			return r, true
		}
	}
	return Runtime{}, false
}
