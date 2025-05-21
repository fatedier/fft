# fft

`fft` is a distributed file transfer tool designed to accelerate the transfer of large files. It achieves this by utilizing multiple relay nodes in parallel, effectively overcoming the bandwidth limitations of a single server.

## Purpose

Transferring large files reliably between two machines, especially when they are behind NATs or firewalls, often requires a relay server with high bandwidth. However, high-bandwidth servers can be expensive.

On the other hand, there are many low-bandwidth servers (e.g., 1MBps) that often sit idle, their resources underutilized.

The goal of `fft` is to leverage these underutilized low-bandwidth servers. By enabling senders and receivers to transfer files through multiple relay nodes simultaneously, `fft` aggregates the bandwidth of these nodes, allowing for faster transfer speeds than would be possible with a single relay. This approach avoids the bottleneck of a single server's bandwidth capacity.

`fft` focuses specifically on the task of file transfer, which typically involves a unidirectional flow of a large amount of data. Once this project matures, the plan is to integrate its capabilities into [frp](https://github.com/fatedier/frp), a fast reverse proxy, to enhance its services by removing bandwidth limitations imposed by a single relay server.

## Architecture

The `fft` system consists of three main components:

*   **`ffts` (Server Control Node):** This is the central coordinator of the system.
    *   It manages the registration of `fftw` worker nodes.
    *   It facilitates the matching of sending and receiving `fft` clients.
    *   When a sender initiates a file transfer, it contacts `ffts`.
    *   When a receiver wants to download a file, it also contacts `ffts` using a transfer ID provided by the sender.
    *   `ffts` then assigns available `fftw` worker nodes to the transfer, enabling the parallel data streams.
    *   `ffts` does not handle any of_the actual file data_ itself; it only manages control signals and metadata.

*   **`fftw` (Worker Node):** These nodes are responsible for relaying the actual file data.
    *   Multiple `fftw` instances can be deployed on various servers.
    *   Each `fftw` registers itself with the `ffts` server, making itself available for relaying transfers.
    *   Upon instruction from `ffts`, an `fftw` node will accept data from a sending `fft` client and forward it to the receiving `fft` client.
    *   The more `fftw` nodes available and assigned to a transfer, the higher the potential aggregate bandwidth and thus faster transfer speeds.

*   **`fft` (Client):** This is the command-line tool used by end-users to send or receive files.
    *   **Sender:** When sending a file, the `fft` client:
        1.  Contacts the `ffts` server to announce a new transfer and receives a unique transfer ID.
        2.  Communicates this transfer ID to the intended recipient (e.g., via email, messaging).
        3.  Upon `ffts` matching it with a receiver and assigning `fftw` nodes, the client splits the file data and sends parts of it in parallel to the assigned `fftw` nodes.
    *   **Receiver:** When receiving a file, the `fft` client:
        1.  Contacts the `ffts` server using the transfer ID obtained from the sender.
        2.  `ffts` matches the receiver with the sender and provides the list of `fftw` nodes involved in the transfer.
        3.  The client then receives data in parallel from these `fftw` nodes and reassembles the original file.

The overall interaction is as follows:
1.  `fftw` nodes start up and register with `ffts`.
2.  An `fft` client (sender) initiates a transfer by contacting `ffts`. `ffts` provides a transfer ID.
3.  The sender shares this transfer ID with another `fft` client (receiver).
4.  The `fft` client (receiver) contacts `ffts` with the transfer ID.
5.  `ffts` matches the sender and receiver and allocates a set of registered `fftw` nodes for the transfer. It informs both clients about these worker nodes.
6.  The `fft` client (sender) then starts sending file data in parallel streams to the allocated `fftw` nodes.
7.  The `fftw` nodes relay this data to the `fft` client (receiver).
8.  The `fft` client (receiver) reassembles the data from the parallel streams to reconstruct the original file.

This architecture allows `fft` to achieve high-speed file transfers by distributing the load across multiple relay servers (`fftw`s), orchestrated by the central `ffts` controller.

## Development Status

`fft` is currently in the early stages of development. Features are still being added, and it is primarily intended for testing purposes.

The `master` branch is used for stable releases, while the `dev` branch is for ongoing development. You can try the latest release for testing.

**The current communication protocol may change at any time and backward compatibility is not guaranteed. Please check the release notes when upgrading to a new version.**

## Usage Example

*   `ffts`: The server control node. Deploy one instance of this.
*   `fftw`: Worker nodes that relay traffic. Deploy multiple instances of these. More worker nodes can increase file transfer speed.
*   `fft`: The client, used for sending and receiving files.

Each program's usage parameters can be viewed by running it with the `-h` flag.

`ffts` and `fftw` need to be deployed on machines with public IP addresses, and their respective ports must be open for `fft` clients to access.

By default, `fftw` and `fft` will attempt to connect to the `ffts` service at `fft.gofrp.org:7777`. If you wish to use your own `ffts` instance, you can specify its address using the `-s {server_addr}` option.

### Sending a File

`./fft -i 123 -l ./filename`

*   `-i 123`: Specifies the transfer request ID. This should be a unique custom value. The receiver will use this ID to accept the file.
*   `-l ./filename`: Specifies the path to the local file to be transferred.

### Receiving a File

`./fft -i 123 -t ./`

*   `-i 123`: Specifies the ID of the transfer request to accept.
*   `-t ./`: Specifies the local path to save the received file. If it's a directory, the sender's original filename will be used and the file saved in that directory. Otherwise, a new file will be created with the specified name.
