# Mobile Architecture for Acorde

Running `acorde` on mobile devices (Android/iOS) requires a different approach than desktop because mobile OSs restrict background processes and do not support standard CLI executables directly (without rooting/jailbreaking).

## High-Level Architecture

Instead of running the `acorde` binary directly, we use **Go Mobile** to compile the core engine into a shared library that is embedded into a native mobile app.

```mermaid
graph TD
    subgraph Mobile_App ["Mobile App (Kotlin/Swift)"]
        UI[Native UI / WebView]
        BG[Background Service]
        
        subgraph Core_Lib ["Acorde Core (Go Shared Lib)"]
            Engine[Engine]
            Sync[P2P Sync]
            API[REST API (Optional)]
        end
        
        UI --> API
        BG --> Sync
    end
```

## 1. The Core Library (`gomobile`)

We use `gomobile bind` to generate Java (Android) and Objective-C (iOS) bindings for the Acorde package.

**Command:**
```bash
gomobile bind -target=android -o acorde.aar ./pkg/mobile
```

We need to create a wrapper package `pkg/mobile` that exposes a simplified API for the native side:
- `StartNode(config)`
- `StopNode()`
- `GetSyncState()`

## 2. Background Execution

### Android
To run in the background (sustain P2P connections), use a **Foreground Service**.
- **Service Type:** `dataSync`
- **Notification:** Must show a persistent notification ("Acorde is syncing...")
- **Battery Optimization:** User may need to disable battery optimization for the app.

### iOS
iOS is more restrictive. True background execution is limited.
- **Background App Refresh:** For periodic short syncs.
- **Background Processing Task:** For heavier maintenance.
- **Push Notifications:** Use "silent push" to wake up the app when new data is available (requires a push server).
- **Foreground:** Sync generally happens while the user has the app open.

## 3. The UI Strategy

Since `acorde` already has a web interface, the easiest mobile UI is a **WebView**.

1. **Native App** starts the Go engine (which binds to `localhost:7331` inside the app sandbox).
2. **Native App** launches a WebView pointing to `http://localhost:7331`.
3. The existing web frontend works exactly as it does on desktop.

## 4. Implementation Steps

1. **Create `pkg/mobile/`**: A Go package designed for export (no `main`, only exported functions).
2. **Build Bindings**:
   ```bash
   gomobile bind -target=android/ios ...
   ```
3. **Android App**:
   - Create new Android Studio project.
   - Import `acorde.aar`.
   - Create a `Service` that calls `acorde.StartNode()`.
   - `MainActivity` contains a `WebView`.
4. **iOS App**:
   - Create Xcode project.
   - Import `Acorde.framework`.
   - `AppDelegate` initializes the engine.

## Termux (Android Power Users)
For a purely text-based/CLI experience on Android, you can use **Termux**:
1. Install Termux from F-Droid.
2. Install Go: `pkg install golang`
3. Clone and build:
   ```bash
   git clone https://github.com/amaydixit11/acorde
   cd acorde
   go build ./cmd/acorde
   ```
4. Run: `./acorde daemon`
