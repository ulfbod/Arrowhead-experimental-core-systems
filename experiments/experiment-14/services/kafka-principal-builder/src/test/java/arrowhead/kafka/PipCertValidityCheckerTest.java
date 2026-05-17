package arrowhead.kafka;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;

import java.io.IOException;
import java.io.OutputStream;
import java.net.ServerSocket;
import java.net.Socket;
import java.nio.charset.StandardCharsets;

import static org.junit.jupiter.api.Assertions.*;

/**
 * PipCertValidityCheckerTest — tests for {@link PipCertValidityChecker} using
 * a ServerSocket-based mock HTTP server (no mocking frameworks, no external libs).
 *
 * <p>Each test spins up a minimal HTTP/1.0 server on a random port, makes a real
 * HTTP call, and verifies the result. The mock server closes after one request.</p>
 */
class PipCertValidityCheckerTest {

    private ServerSocket serverSocket;
    private int port;
    private Thread serverThread;

    @BeforeEach
    void startMockServer() throws IOException {
        serverSocket = new ServerSocket(0); // OS assigns a free port
        port = serverSocket.getLocalPort();
    }

    @AfterEach
    void stopMockServer() throws IOException {
        serverSocket.close();
        if (serverThread != null) {
            serverThread.interrupt();
        }
    }

    /**
     * Starts a single-request mock HTTP server that responds with the given status
     * code and body.
     */
    private void serveSingleResponse(int statusCode, String body) {
        serverThread = new Thread(() -> {
            try (Socket client = serverSocket.accept()) {
                // Drain the request
                byte[] buf = new byte[4096];
                client.getInputStream().read(buf);

                String statusText = statusCode == 200 ? "OK" : "Not Found";
                String response = "HTTP/1.0 " + statusCode + " " + statusText + "\r\n" +
                    "Content-Type: application/json\r\n" +
                    "Content-Length: " + body.getBytes(StandardCharsets.UTF_8).length + "\r\n" +
                    "\r\n" +
                    body;
                OutputStream out = client.getOutputStream();
                out.write(response.getBytes(StandardCharsets.UTF_8));
                out.flush();
            } catch (IOException e) {
                // Ignore — test teardown closes the socket
            }
        });
        serverThread.setDaemon(true);
        serverThread.start();
    }

    @Test
    void validCert_returnsTrue() throws Exception {
        String body = "{\"systemName\":\"test-system\",\"certLevel\":\"sy\",\"valid\":true}";
        serveSingleResponse(200, body);

        PipCertValidityChecker checker = new PipCertValidityChecker("http://127.0.0.1:" + port);
        assertTrue(checker.isCertValid("test-system"),
            "valid=true in PIP response should return true");
    }

    @Test
    void revokedCert_returnsFalse() throws Exception {
        String body = "{\"systemName\":\"revoked-system\",\"certLevel\":\"sy\",\"valid\":false}";
        serveSingleResponse(200, body);

        PipCertValidityChecker checker = new PipCertValidityChecker("http://127.0.0.1:" + port);
        assertFalse(checker.isCertValid("revoked-system"),
            "valid=false in PIP response should return false");
    }

    @Test
    void unknownCn_404_returnsFalse() throws Exception {
        serveSingleResponse(404, "{}");

        PipCertValidityChecker checker = new PipCertValidityChecker("http://127.0.0.1:" + port);
        assertFalse(checker.isCertValid("unknown-cn"),
            "404 from PIP should return false (fail-closed)");
    }

    @Test
    void pipUnreachable_returnsFalse() {
        // Use an unroutable address (port 1 on loopback)
        PipCertValidityChecker checker = new PipCertValidityChecker("http://127.0.0.1:1");
        assertFalse(checker.isCertValid("any-cn"),
            "Unreachable PIP should return false (fail-closed)");
    }

    @Test
    void malformedJson_returnsFalse() throws Exception {
        serveSingleResponse(200, "not-json-at-all");

        PipCertValidityChecker checker = new PipCertValidityChecker("http://127.0.0.1:" + port);
        assertFalse(checker.isCertValid("test"),
            "Malformed JSON from PIP should return false (fail-closed)");
    }

    @Test
    void missingValidField_returnsFalse() throws Exception {
        String body = "{\"systemName\":\"test\",\"certLevel\":\"sy\"}"; // no "valid" field
        serveSingleResponse(200, body);

        PipCertValidityChecker checker = new PipCertValidityChecker("http://127.0.0.1:" + port);
        assertFalse(checker.isCertValid("test"),
            "Missing 'valid' field in PIP response should return false (fail-closed)");
    }

    @Test
    void trailingSlashInBaseUrl_handledCorrectly() throws Exception {
        String body = "{\"systemName\":\"test\",\"certLevel\":\"sy\",\"valid\":true}";
        serveSingleResponse(200, body);

        // Base URL with trailing slash should not produce double-slash in path
        PipCertValidityChecker checker = new PipCertValidityChecker("http://127.0.0.1:" + port + "/");
        assertTrue(checker.isCertValid("test"),
            "Trailing slash in base URL should be stripped correctly");
    }
}
