# Discovery Strategy

How do vaultd nodes find each other?

We use a hybrid approach combining **Local Multicast** for speed and **Global DHT** for reachability.

## 1. Local LAN: mDNS (Zero-Conf)
On a local network (Home WiFi), devices shouldn't need internet to sync.

-   **Protocol**: Multicast DNS (RFC 6762).
-   **Service**: `_vaultd._tcp`.
-   **Mechanism**:
    1.  Node A broadcasts "Who is `_vaultd`?".
    2.  Node B responds "I am, at IP 192.168.1.5:4001, PeerID=QmXYZ...".
    3.  Node A connects directly.
-   **Pros**: Extremely fast (<100ms), works without internet.
-   **Cons**: Only works within the same subnet.

## 2. Wide Area: Kademlia DHT
When devices are on different networks (e.g., Phone on 5G, Laptop on Home WiFi), we need a rendezvous point.

-   **Protocol**: Kademlia Distributed Hash Table (libp2p).
-   **Namespace**: `/vaultd/1.0.0`.
-   **Mechanism**:
    1.  Node A (Laptop) announces itself: "I am PeerID QmLaptop... provide me!". The DHT stores this record on random nodes in the network.
    2.  Node B (Phone) searches: "Where is PeerID QmLaptop?".
    3.  The DHT responds with Node A's public IP (and NAT traversal info).
    4.  Node B attempts connection (using Hole Punching / NAT traversal).
-   **Pros**: Works globally.
-   **Cons**: Slower (seconds), requires some bootstrap nodes (we use generic IPFS/libp2p bootstrappers for now).

## 3. Direct Pairing
For cases where discovery fails or you want strict control, you can manually pair.

-   **Invite Link**: `vaultd://<PeerID>@<IP>:<Port>?key=<Key>`
-   Bypasses discovery lookup and dials the address directly.
