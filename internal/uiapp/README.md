# Native workspace boundary

`App.Start(window.Update)` dispatches background results onto the native UI thread. Workspaces expose `View(*ui.Context)`, own only selection/drafts/feedback, and call typed services. Add future workspace navigation in `app.go`; keep domain UI in separate files.

`storage.Open(storage.PathsFromHome(home))` opens the compatible shared schema. `--data-dir DIR` instead isolates **both** database (`DIR/.agent-manager/agent-manager.db`) and generated documents (`DIR/.agents/AGENTS.md`). All automated checks use temporary directories. Legacy Skills JSON remains untouched until the Skills service handles its catalog/link migration; AGENTS.md startup does not inspect GAAL or Agent caches.

From the repository root:

```sh
go run ./cmd/agent-manager --data-dir /tmp/agent-manager-validation
go test ./...
go vet ./...
go build ./cmd/agent-manager
```

Existing Svelte/Tauri implementation and independent `native-sample` remain until later migration tickets are complete.
