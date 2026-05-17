package arrowhead.kafka;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

import static org.junit.jupiter.api.Assertions.*;

/**
 * CnExtractionTest — pure unit tests for CN extraction from Distinguished Name strings.
 *
 * <p>These tests exercise {@link ArrowheadPrincipalBuilder#extractCN(String)} without
 * any mocking or HTTP calls. The method must handle RFC 2253 format (commas without
 * spaces) and legacy format (commas with spaces).</p>
 */
class CnExtractionTest {

    @ParameterizedTest(name = "[{index}] dn={0} → cn={1}")
    @CsvSource({
        // RFC 2253 format: no spaces after commas
        "CN=portal-cloud-ml,OU=sy,O=ArrowheadCloud,            portal-cloud-ml",
        "CN=service-partner-1,OU=sy,O=ArrowheadCloud,          service-partner-1",
        "CN=robot-fleet-site-1,OU=sy,O=ArrowheadCloud,         robot-fleet-site-1",
        // Legacy format: spaces after commas
        "CN=test-system, OU=de, O=ArrowheadCloud,              test-system",
        "CN=kafka, OU=sy, O=ArrowheadCloud,                    kafka",
        // CN at end
        "O=ArrowheadCloud,OU=sy,CN=deep-cn,                    deep-cn",
        // CN with hyphen and numbers
        "CN=robot-001,OU=sy,                                   robot-001",
    })
    void extractCN_variousDnFormats(String dn, String expectedCn) {
        String actual = ArrowheadPrincipalBuilder.extractCN(dn.trim());
        assertEquals(expectedCn.trim(), actual,
            "extractCN(\"" + dn.trim() + "\") should return \"" + expectedCn.trim() + "\"");
    }

    @Test
    void extractCN_nullDn_returnsNull() {
        assertNull(ArrowheadPrincipalBuilder.extractCN(null));
    }

    @Test
    void extractCN_emptyDn_returnsNull() {
        assertNull(ArrowheadPrincipalBuilder.extractCN(""));
    }

    @Test
    void extractCN_noCnAttribute_returnsNull() {
        assertNull(ArrowheadPrincipalBuilder.extractCN("OU=sy,O=ArrowheadCloud"));
    }

    @Test
    void extractCN_onlyCn_returnsCnValue() {
        assertEquals("simple-system", ArrowheadPrincipalBuilder.extractCN("CN=simple-system"));
    }

    @Test
    void extractCN_caseInsensitive() {
        // The Kafka/Java JCA stack uses uppercase CN= but we guard against lowercase too.
        assertEquals("test", ArrowheadPrincipalBuilder.extractCN("cn=test,OU=sy"));
    }
}
