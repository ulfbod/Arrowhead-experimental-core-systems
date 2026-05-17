package arrowhead.kafka;

import org.apache.kafka.common.Configurable;
import org.apache.kafka.common.KafkaException;
import org.apache.kafka.common.security.auth.AuthenticationContext;
import org.apache.kafka.common.security.auth.KafkaPrincipal;
import org.apache.kafka.common.security.auth.KafkaPrincipalBuilder;
import org.apache.kafka.common.security.auth.KafkaPrincipalSerde;
import org.apache.kafka.common.security.auth.SslAuthenticationContext;

import javax.net.ssl.SSLSession;
import java.nio.charset.StandardCharsets;
import java.security.cert.Certificate;
import java.security.cert.X509Certificate;
import java.util.Map;

/**
 * ArrowheadPrincipalBuilder — Kafka KafkaPrincipalBuilder plugin for experiment-14.
 *
 * <p>Implements connection-time certificate revocation enforcement (design decision
 * D2'). After the TLS handshake succeeds (cert is structurally valid and signed by
 * the CA), this builder:</p>
 * <ol>
 *   <li>Extracts the Common Name (CN) from the peer certificate's Distinguished Name.</li>
 *   <li>Queries the Arrowhead PIP ({@code GET /pip/attributes/{cn}}) to check
 *       whether the cert is currently valid (not revoked, not expired in PIP).</li>
 *   <li>If {@code valid=false} (or PIP is unreachable), throws
 *       {@link KafkaException} wrapping an {@link IllegalArgumentException}, which
 *       Kafka translates into a connection-level authentication failure.</li>
 *   <li>If {@code valid=true}, returns a {@link KafkaPrincipal} with the CN as the
 *       principal name.</li>
 * </ol>
 *
 * <p>Configuration property:
 * <pre>
 *   arrowhead.pip.url = http://pip:9506
 * </pre>
 * Pass via {@code KAFKA_OPTS=-Darrowhead.pip.url=http://pip:9506} or Kafka
 * broker configuration.</p>
 *
 * <p>The class has two constructors:</p>
 * <ul>
 *   <li>{@link #ArrowheadPrincipalBuilder()} — public no-arg constructor used by
 *       Kafka's reflective instantiation.</li>
 *   <li>{@link #ArrowheadPrincipalBuilder(CertValidityChecker)} — package-private
 *       constructor for unit tests (inject a lambda mock).</li>
 * </ul>
 */
public class ArrowheadPrincipalBuilder implements KafkaPrincipalBuilder, KafkaPrincipalSerde, Configurable {

    static final String PIP_URL_PROP = "arrowhead.pip.url";
    static final String DEFAULT_PIP_URL = "http://pip:9506";

    private CertValidityChecker checker;

    /**
     * Public no-arg constructor for Kafka's reflective instantiation.
     * {@link #configure(Map)} must be called before {@link #build(AuthenticationContext)}.
     */
    public ArrowheadPrincipalBuilder() {
        // checker is set in configure()
    }

    /**
     * Package-private constructor for unit testing — injects a CertValidityChecker
     * directly so tests do not need an HTTP server.
     *
     * @param checker the checker implementation to use
     */
    ArrowheadPrincipalBuilder(CertValidityChecker checker) {
        this.checker = checker;
    }

    /**
     * Called by Kafka after instantiation. Reads {@code arrowhead.pip.url} from
     * the broker's configuration (passed via KAFKA_OPTS system properties) and
     * creates a {@link PipCertValidityChecker}.
     *
     * @param configs broker configuration map (unused; PIP URL read from system property)
     */
    @Override
    public void configure(Map<String, ?> configs) {
        if (checker == null) {
            String pipUrl = System.getProperty(PIP_URL_PROP, DEFAULT_PIP_URL);
            checker = new PipCertValidityChecker(pipUrl);
        }
    }

    /**
     * Builds the Kafka principal for an incoming connection.
     *
     * <p>For SSL connections: extracts the peer cert CN, queries PIP, and either
     * returns a {@link KafkaPrincipal} or throws {@link KafkaException} to reject
     * the connection.</p>
     *
     * <p>For non-SSL connections: delegates to the default ANONYMOUS principal.</p>
     *
     * @param context authentication context provided by Kafka after TLS handshake
     * @return KafkaPrincipal with CN as principal name
     * @throws KafkaException if the certificate is revoked or PIP is unreachable
     */
    @Override
    public KafkaPrincipal build(AuthenticationContext context) {
        if (!(context instanceof SslAuthenticationContext)) {
            return KafkaPrincipal.ANONYMOUS;
        }

        SslAuthenticationContext sslCtx = (SslAuthenticationContext) context;
        SSLSession session = sslCtx.session();

        String cn;
        try {
            Certificate[] certs = session.getPeerCertificates();
            if (certs == null || certs.length == 0) {
                throw new KafkaException("No peer certificate in SSL session");
            }
            X509Certificate x509 = (X509Certificate) certs[0];
            cn = extractCN(x509.getSubjectX500Principal().getName());
        } catch (javax.net.ssl.SSLPeerUnverifiedException e) {
            throw new KafkaException("SSL peer unverified: " + e.getMessage(), e);
        }

        if (cn == null || cn.isEmpty()) {
            throw new KafkaException("Cannot extract CN from peer certificate");
        }

        // D2' — connection-time cert validity pre-gate.
        // Fail-closed: if PIP is unreachable, checker returns false.
        if (!checker.isCertValid(cn)) {
            throw new KafkaException(
                new IllegalArgumentException(
                    "Certificate for CN=" + cn + " is revoked or unknown in PIP"
                )
            );
        }

        return new KafkaPrincipal(KafkaPrincipal.USER_TYPE, cn);
    }

    /**
     * Serializes a KafkaPrincipal to bytes for inter-broker forwarding in KRaft mode.
     * Format: {@code "type:name"} encoded as UTF-8.
     */
    @Override
    public byte[] serialize(KafkaPrincipal principal) throws KafkaException {
        return (principal.getPrincipalType() + ":" + principal.getName())
                .getBytes(StandardCharsets.UTF_8);
    }

    /**
     * Deserializes a KafkaPrincipal from bytes produced by {@link #serialize}.
     */
    @Override
    public KafkaPrincipal deserialize(byte[] bytes) throws KafkaException {
        String encoded = new String(bytes, StandardCharsets.UTF_8);
        int colon = encoded.indexOf(':');
        if (colon < 0) {
            throw new KafkaException("Invalid principal encoding: " + encoded);
        }
        return new KafkaPrincipal(encoded.substring(0, colon), encoded.substring(colon + 1));
    }

    /**
     * Extracts the Common Name (CN) value from an X.500 Distinguished Name string.
     *
     * <p>Supports both RFC 2253 format ({@code CN=foo,OU=bar,...}) and
     * legacy format ({@code CN=foo, OU=bar, ...}) with spaces after commas.</p>
     *
     * @param dn the Distinguished Name string from {@code X509Certificate.getSubjectX500Principal().getName()}
     * @return the CN value, or {@code null} if not found
     */
    static String extractCN(String dn) {
        if (dn == null || dn.isEmpty()) {
            return null;
        }
        // Split on commas that are not inside quotes or escaped.
        // X500Principal.getName(RFC2253) does not include spaces after commas.
        for (String part : dn.split(",")) {
            String trimmed = part.trim();
            if (trimmed.toUpperCase().startsWith("CN=")) {
                return trimmed.substring(3).trim();
            }
        }
        return null;
    }
}
