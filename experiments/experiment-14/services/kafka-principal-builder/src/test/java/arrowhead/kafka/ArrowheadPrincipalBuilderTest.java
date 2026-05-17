package arrowhead.kafka;

import org.apache.kafka.common.KafkaException;
import org.apache.kafka.common.security.auth.KafkaPrincipal;
import org.apache.kafka.common.security.auth.SslAuthenticationContext;
import org.junit.jupiter.api.Test;

import javax.net.ssl.SSLSession;
import java.net.InetAddress;
import java.security.cert.Certificate;
import java.security.cert.X509Certificate;
import java.util.Collections;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * ArrowheadPrincipalBuilderTest — unit tests using lambda mocks for
 * {@link CertValidityChecker}. No HTTP server required.
 *
 * <p>Since we cannot easily create real X.509 certs in unit tests, we use Mockito
 * to mock SSLSession and X509Certificate. The pom.xml includes Mockito via the
 * mockito-core dependency added for these tests.</p>
 *
 * <p>Tests cover:</p>
 * <ul>
 *   <li>Valid cert → principal returned with correct CN</li>
 *   <li>Revoked cert (checker returns false) → KafkaException thrown</li>
 *   <li>PIP unreachable (checker returns false) → KafkaException thrown (fail-closed)</li>
 *   <li>configure() with system property sets PIP URL</li>
 * </ul>
 */
class ArrowheadPrincipalBuilderTest {

    /**
     * Creates a mock SslAuthenticationContext whose peer certificate has the given DN.
     */
    private static SslAuthenticationContext mockSslCtx(String dn) throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        javax.security.auth.x500.X500Principal principal =
            new javax.security.auth.x500.X500Principal(dn);
        when(cert.getSubjectX500Principal()).thenReturn(principal);

        SSLSession session = mock(SSLSession.class);
        when(session.getPeerCertificates()).thenReturn(new Certificate[]{cert});

        SslAuthenticationContext ctx = mock(SslAuthenticationContext.class);
        when(ctx.session()).thenReturn(session);
        return ctx;
    }

    @Test
    void build_validCert_returnsPrincipalWithCn() throws Exception {
        // checker returns true → connection accepted
        ArrowheadPrincipalBuilder builder = new ArrowheadPrincipalBuilder(cn -> true);

        SslAuthenticationContext ctx = mockSslCtx("CN=portal-cloud-ml,OU=sy,O=ArrowheadCloud");
        KafkaPrincipal result = builder.build(ctx);

        assertEquals(KafkaPrincipal.USER_TYPE, result.getPrincipalType());
        assertEquals("portal-cloud-ml", result.getName());
    }

    @Test
    void build_revokedCert_throwsKafkaException() throws Exception {
        // checker returns false → connection rejected
        ArrowheadPrincipalBuilder builder = new ArrowheadPrincipalBuilder(cn -> false);

        SslAuthenticationContext ctx = mockSslCtx("CN=revoked-system,OU=sy,O=ArrowheadCloud");

        KafkaException ex = assertThrows(KafkaException.class, () -> builder.build(ctx));
        assertTrue(ex.getCause() instanceof IllegalArgumentException,
            "Cause should be IllegalArgumentException; got: " + ex.getCause());
        assertTrue(ex.getCause().getMessage().contains("revoked-system"),
            "Exception message should mention the CN");
    }

    @Test
    void build_pipUnreachable_failClosed() throws Exception {
        // Simulates PIP unreachable: checker returns false (fail-closed contract)
        ArrowheadPrincipalBuilder builder = new ArrowheadPrincipalBuilder(cn -> false);

        SslAuthenticationContext ctx = mockSslCtx("CN=consumer-1,OU=sy,O=ArrowheadCloud");

        assertThrows(KafkaException.class, () -> builder.build(ctx),
            "PIP unreachable must cause connection rejection (fail-closed)");
    }

    @Test
    void build_certCnPassedToChecker() throws Exception {
        // Verify the checker receives the exact CN extracted from the cert DN.
        String[] receivedCn = {null};
        ArrowheadPrincipalBuilder builder = new ArrowheadPrincipalBuilder(cn -> {
            receivedCn[0] = cn;
            return true;
        });

        SslAuthenticationContext ctx = mockSslCtx("CN=specific-system,OU=sy,O=Test");
        builder.build(ctx);

        assertEquals("specific-system", receivedCn[0],
            "checker must receive the exact CN from the certificate");
    }

    @Test
    void configure_createsPipChecker() {
        // configure() with no system property should not throw.
        ArrowheadPrincipalBuilder builder = new ArrowheadPrincipalBuilder();
        // Does not throw; checker is created lazily in configure().
        assertDoesNotThrow(() -> builder.configure(Collections.emptyMap()));
    }

    @Test
    void configure_usesPipUrlSystemProperty() {
        System.setProperty(ArrowheadPrincipalBuilder.PIP_URL_PROP, "http://localhost:1");
        try {
            ArrowheadPrincipalBuilder builder = new ArrowheadPrincipalBuilder();
            builder.configure(Collections.emptyMap());
            // Builder created without exception; PipCertValidityChecker will fail on
            // actual calls (unreachable port 1), but construction is fine.
        } finally {
            System.clearProperty(ArrowheadPrincipalBuilder.PIP_URL_PROP);
        }
    }
}
