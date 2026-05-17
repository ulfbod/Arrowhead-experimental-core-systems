package arrowhead.kafka;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.IOException;
import java.net.HttpURLConnection;
import java.net.URL;

/**
 * PipCertValidityChecker — HTTP client that queries the Arrowhead PIP for
 * certificate validity at connection time.
 *
 * <p>Calls {@code GET {pipBaseUrl}/pip/attributes/{cn}} and reads the {@code valid}
 * boolean field from the JSON response. Fails closed on any error: network
 * failures, non-200 status codes, missing fields, and JSON parse errors all
 * return {@code false}.</p>
 *
 * <p>This is the production implementation of {@link CertValidityChecker} used
 * by {@link ArrowheadPrincipalBuilder} in the running Kafka broker.</p>
 */
public class PipCertValidityChecker implements CertValidityChecker {

    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final int CONNECT_TIMEOUT_MS = 3_000;
    private static final int READ_TIMEOUT_MS    = 3_000;

    private final String pipBaseUrl;

    /**
     * @param pipBaseUrl base URL of the PIP service, e.g. {@code http://pip:9506}
     */
    public PipCertValidityChecker(String pipBaseUrl) {
        this.pipBaseUrl = pipBaseUrl.replaceAll("/$", "");
    }

    /**
     * Queries {@code GET /pip/attributes/{cn}} and returns the {@code valid} field.
     *
     * <p>Fail-closed: any error (network, non-200, missing field, parse error)
     * returns {@code false}.</p>
     *
     * @param cn the certificate Common Name to check
     * @return {@code true} if {@code valid=true} in the PIP response
     */
    @Override
    public boolean isCertValid(String cn) {
        try {
            String urlStr = pipBaseUrl + "/pip/attributes/" + cn;
            HttpURLConnection conn = (HttpURLConnection) new URL(urlStr).openConnection();
            conn.setConnectTimeout(CONNECT_TIMEOUT_MS);
            conn.setReadTimeout(READ_TIMEOUT_MS);
            conn.setRequestMethod("GET");
            conn.setRequestProperty("Accept", "application/json");

            int status = conn.getResponseCode();
            if (status != 200) {
                return false; // 404 (unknown CN) or any other error → fail-closed
            }

            JsonNode root = MAPPER.readTree(conn.getInputStream());
            JsonNode validNode = root.get("valid");
            if (validNode == null || !validNode.isBoolean()) {
                return false; // missing or non-boolean field → fail-closed
            }
            return validNode.asBoolean();

        } catch (IOException e) {
            // Network error, timeout, connection refused → fail-closed
            return false;
        }
    }
}
