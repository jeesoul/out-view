package com.outview.webrtc;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.newsclub.net.unix.AFUNIXSocket;
import org.newsclub.net.unix.AFUNIXSocketAddress;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.*;
import java.net.Socket;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.function.Consumer;

/**
 * Manages a single IPC connection to the WebRTC Sidecar.
 * Protocol: 4-byte big-endian length prefix + JSON payload.
 * On Windows, falls back to TCP at 127.0.0.1:9999.
 */
public class IPCConnection implements Closeable {
    private static final Logger log = LoggerFactory.getLogger(IPCConnection.class);

    private final ObjectMapper mapper = new ObjectMapper();
    private final CopyOnWriteArrayList<Consumer<IPCMessage>> listeners = new CopyOnWriteArrayList<>();
    private final AtomicBoolean closed = new AtomicBoolean(false);

    private int maxMessageSize = 4 * 1024 * 1024; // default, overridden in connect

    private Socket socket;
    private OutputStream out;
    private InputStream in;
    private Thread readerThread;

    /**
     * Connect to the Sidecar. Uses Unix socket on Linux/macOS, TCP fallback on Windows.
     */
    public void connect(String socketPath, int connectTimeoutMs, int readTimeoutMs, int maxMessageSize) throws IOException {
        this.maxMessageSize = maxMessageSize;
        String os = System.getProperty("os.name", "").toLowerCase();
        if (os.contains("win")) {
            // Windows: named pipe support requires go-winio on the sidecar side.
            // For now, use TCP fallback. The sidecar should be configured with -tcp-addr=127.0.0.1:9999
            Socket tcpSocket = new Socket();
            tcpSocket.connect(new java.net.InetSocketAddress("127.0.0.1", 9999), connectTimeoutMs);
            tcpSocket.setSoTimeout(readTimeoutMs);
            this.socket = tcpSocket;
        } else {
            AFUNIXSocket unixSocket = AFUNIXSocket.newInstance();
            unixSocket.connect(AFUNIXSocketAddress.of(new File(socketPath)), connectTimeoutMs);
            unixSocket.setSoTimeout(readTimeoutMs);
            this.socket = unixSocket;
        }
        this.out = new BufferedOutputStream(socket.getOutputStream());
        this.in = new BufferedInputStream(socket.getInputStream());
        startReaderThread();
        log.info("IPC connected to sidecar via {}", os.contains("win") ? "TCP" : "Unix socket");
    }

    /**
     * Send a message to the Sidecar (thread-safe).
     */
    public synchronized void send(IPCMessage message) throws IOException {
        if (closed.get()) throw new IOException("connection closed");
        byte[] json = mapper.writeValueAsBytes(message);
        ByteBuffer lenBuf = ByteBuffer.allocate(4).order(ByteOrder.BIG_ENDIAN);
        lenBuf.putInt(json.length);
        out.write(lenBuf.array());
        out.write(json);
        out.flush();
    }

    /** Register a listener for incoming messages. */
    public void addListener(Consumer<IPCMessage> listener) {
        listeners.add(listener);
    }

    /** Remove a listener. */
    public void removeListener(Consumer<IPCMessage> listener) {
        listeners.remove(listener);
    }

    @Override
    public void close() {
        if (closed.compareAndSet(false, true)) {
            if (readerThread != null) {
                readerThread.interrupt();
            }
            try {
                if (socket != null) socket.close();
            } catch (IOException e) {
                log.debug("Error closing socket", e);
            }
        }
    }

    public boolean isClosed() {
        return closed.get();
    }

    private void startReaderThread() {
        readerThread = new Thread(new Runnable() {
            @Override
            public void run() {
                readLoop();
            }
        }, "ipc-reader");
        readerThread.setDaemon(true);
        readerThread.start();
    }

    private void readLoop() {
        byte[] lenBuf = new byte[4];
        while (!closed.get()) {
            try {
                // Read 4-byte length prefix
                int read = 0;
                while (read < 4) {
                    int n = in.read(lenBuf, read, 4 - read);
                    if (n < 0) {
                        log.info("IPC connection closed by sidecar");
                        close();
                        return;
                    }
                    read += n;
                }
                int length = ByteBuffer.wrap(lenBuf).order(ByteOrder.BIG_ENDIAN).getInt();
                if (length <= 0 || length > maxMessageSize) {
                    log.error("Invalid IPC message length: {}", length);
                    close();
                    return;
                }

                // Read JSON payload
                byte[] payload = new byte[length];
                int payloadRead = 0;
                while (payloadRead < length) {
                    int n = in.read(payload, payloadRead, length - payloadRead);
                    if (n < 0) {
                        log.info("IPC connection closed mid-message");
                        close();
                        return;
                    }
                    payloadRead += n;
                }

                IPCMessage msg = mapper.readValue(payload, IPCMessage.class);
                for (Consumer<IPCMessage> listener : listeners) {
                    try {
                        listener.accept(msg);
                    } catch (Exception e) {
                        log.error("IPC listener error", e);
                    }
                }
            } catch (IOException e) {
                if (!closed.get()) {
                    log.error("IPC read error", e);
                    close();
                }
                return;
            }
        }
    }
}
