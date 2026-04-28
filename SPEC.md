# Service Registry Specification (Expanded & Structured)

## Overview

The Service Registry is an Arrowhead Core System responsible for:

- Registering service providers
- Storing service metadata
- Enabling service discovery via queries

This specification defines a **functional subset aligned with the official Arrowhead specification**, including metadata, interfaces, and matching behavior.

---

## Core Concepts

### System

Represents a provider of services.

Fields:
- systemName (string, required, non-empty)
- address (string, required, non-empty)
- port (integer, required, >0)

Optional:
- authenticationInfo (string)

Uniqueness:
- A system is uniquely identified by:
  `(systemName, address, port)`

---

### Service Definition

Represents a logical service.

Fields:
- serviceDefinition (string, required, non-empty)

---

### Service Instance

Represents a registered service provided by a system.

Fields:
- serviceDefinition (string, required)
- providerSystem (System, required)
- serviceUri (string, required)
- interfaces (list of strings, required, non-empty)
- version (integer, optional, default = 1)
- metadata (map<string,string>, optional)
- secure (string, optional, e.g. "NOT_SECURE", "CERTIFICATE")

---

## API Endpoints

---

### 1. Register Service

**POST** `/serviceregistry/register`

#### Request
```json
{
  "serviceDefinition": "string",
  "providerSystem": {
    "systemName": "string",
    "address": "string",
    "port": 0,
    "authenticationInfo": "string"
  },
  "serviceUri": "string",
  "interfaces": ["string"],
  "version": 1,
  "metadata": {
    "key": "value"
  },
  "secure": "NOT_SECURE"
}