package arrowhead.kafka;

/**
 * CertValidityChecker — functional interface for checking certificate validity.
 *
 * <p>Abstracts the PIP HTTP call so that tests can inject a lambda mock without
 * standing up an HTTP server. The production implementation is
 * {@link PipCertValidityChecker}.</p>
 *
 * <p>Contract: returns {@code false} on any error (fail-closed). Implementations
 * must never throw checked exceptions — errors are suppressed and treated as
 * cert-invalid.</p>
 */
@FunctionalInterface
public interface CertValidityChecker {
    /**
     * Returns {@code true} if the certificate for {@code cn} is currently valid
     * (not revoked, not expired) according to the PIP, or {@code false} if the
     * cert is revoked, unknown, or if the PIP is unreachable.
     *
     * @param cn the certificate Common Name to check
     * @return {@code true} if cert is valid, {@code false} otherwise (fail-closed)
     */
    boolean isCertValid(String cn);
}
