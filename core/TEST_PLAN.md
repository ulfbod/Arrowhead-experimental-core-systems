# Test Plan

## Goal

Verify full compliance with SPEC.md.

---

## Registration Tests

### Valid Registration
- Expect: 201 Created

### Missing Fields
- Missing serviceDefinition → 400
- Missing providerSystem → 400
- Missing interfaces → 400

---

### Duplicate Registration

- Register same service twice
- Expect:
  - Overwrites previous entry
  - No duplicates stored

---

### Metadata Handling

- Register with metadata
- Retrieve and verify metadata is preserved

---

### Version Handling

- Register version = 1
- Register version = 2 (same service)
- Query with versionRequirement = 2 → must match correct one

---

## Query Tests

### Exact Match
- Register + query → success

---

### Interface Matching

- Register with:
  ["HTTP", "HTTPS"]

- Query with:
  ["HTTPS"] → match
  ["COAP"] → no match

---

### Metadata Matching

- Register metadata:
  { "region": "eu" }

- Query:
  { "region": "eu" } → match
  { "region": "us" } → no match

---

### Version Matching

- Register version 1 and 2
- Query versionRequirement = 2 → only version 2 returned

---

### No Match

- Query unknown service → empty list

---

## Integration Tests

### Full Flow

1. Register service
2. Query service
3. Validate returned structure

---

## Error Handling

- Invalid JSON → 400
- Wrong types → 400

---

## Edge Cases

- Multiple interfaces
- Empty metadata
- Missing optional fields
- Duplicate overwrite behavior

---

## Test Principles

- Deterministic
- Table-driven where possible
- Minimal mocking