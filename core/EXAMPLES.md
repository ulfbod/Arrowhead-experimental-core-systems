# Examples

---

## Example 1: Full Registration

```json
{
  "serviceDefinition": "temperature-service",
  "providerSystem": {
    "systemName": "sensor-1",
    "address": "192.168.0.10",
    "port": 8080
  },
  "serviceUri": "/temperature",
  "interfaces": ["HTTP-SECURE-JSON"],
  "version": 1,
  "metadata": {
    "region": "eu",
    "unit": "celsius"
  },
  "secure": "NOT_SECURE"
}