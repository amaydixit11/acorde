# Running ACORDE on Android via Termux

Since ACORDE is a standard Go application, it runs perfectly on Android using [Termux](https://termux.dev).

## Prerequisites

1.  **Install Termux**
    *   **Recommended**: Install from [F-Droid](https://f-droid.org/en/packages/com.termux/).
    *   *Note: The Google Play Store version is outdated and may not work.*

## Installation Steps (On your Phone)

Open the Termux app and run the following commands:

### 1. Update Packages
```bash
pkg update && pkg upgrade
```

### 2. Install Go and Git
```bash
pkg install golang git
```

### 3. Clone the Repository
```bash
git clone https://github.com/amaydixit11/acorde.git
cd acorde
```

### 4. Build ACORDE
```bash
cd cmd/acorde
go build -o acorde .
```

### 5. Verify Installation
```bash
./acorde help
```

## Troubleshooting

### "invalid reference to net.zoneCache" Error

This error happens because the installed Go version (likely 1.23+) is too new for the networking library used (`anet`). The most reliable fix in Termux is to manually patch the library file.

1.  **Make the file writable**:
    ```bash
    chmod +w ~/go/pkg/mod/github.com/wlynxg/anet@v0.0.5/interface_android.go
    ```

2.  **Edit the file**:
    ```bash
    nano ~/go/pkg/mod/github.com/wlynxg/anet@v0.0.5/interface_android.go
    ```

3.  **Find and Comment Out Code**:
    Find the lines referencing `zoneCache` (usually `//go:linkname` and the `var zoneCache` line).
    **Comment them out** (add `//` at the start of the line).

    Also find the function that uses `zoneCache`. It usually looks like:
    ```go
    func (i *Interface) linkLocal() ... {
        // ... code using zoneCache ...
    }
    ```
    Change the function body to just return `nil` or an empty result if possible, or comment out the `zoneCache` logic inside it. 
    
    *Note: This disables some Android-specific network optimizations but allows the app to compile and run.*

4.  **Rebuild**:
    ```bash
    go build -o acorde ./cmd/acorde
    ```

## Running the Daemon

To start the sync daemon on your phone:

```bash
# Create a data directory
mkdir -p ~/acorde-data

# Start daemon
./acorde daemon --data ~/acorde-data --port 4001 --api-port 7331
```

## Pairing with your Laptop

1.  **On your Laptop**: Generate an invite code.
    ```bash
    ./acorde invite --data ./data/minelab
    ```

2.  **On your Phone (Termux)**: Pair using the code.
    ```bash
    ./acorde pair "acorde://..." --data ~/acorde-data
    ```

## Notes

*   **Background Running**: Termux may be killed by Android's battery optimizer. You should "Acquire Wakelock" from the Termux notification or disable battery optimizations for Termux in Android settings.
*   **Networking**: Both devices must be on the **same WiFi network** for mDNS discovery to work automatically.
