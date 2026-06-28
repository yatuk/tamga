"""Privacy entity coverage matrix — GDPR, KVKK, HIPAA, PCI-DSS."""

from dataclasses import dataclass


@dataclass
class PrivacyEntity:
    entity: str
    regulation: str  # "GDPR", "KVKK", "HIPAA", "PCI-DSS"
    tamga_supported: bool
    scanner: str = ""
    notes: str = ""


PRIVACY_ENTITIES: list[PrivacyEntity] = [
    # GDPR
    PrivacyEntity("Email address", "GDPR", True, "pii_scanner", ""),
    PrivacyEntity("Phone number", "GDPR", True, "pii_scanner", "TR phone format supported"),
    PrivacyEntity("IP address (public)", "GDPR", True, "pii_scanner", "v4 + v6"),
    PrivacyEntity("Name / surname", "GDPR", True, "pii_deep", "TR name hints via sidecar"),
    PrivacyEntity("Physical address", "GDPR", True, "pii_deep", "TR address hints via sidecar"),
    PrivacyEntity("Date of birth", "GDPR", True, "pii_deep", "Presidio regex recognizer"),
    PrivacyEntity("Passport number", "GDPR", True, "pii_deep", "Presidio regex recognizer"),
    PrivacyEntity("National ID number", "GDPR", True, "pii_deep", "Presidio regex recognizer (non-TR formats)"),
    # KVKK (TR-specific)
    PrivacyEntity("TC Kimlik No", "KVKK", True, "pii_scanner + pii_deep", "MERNIS-validated"),
    PrivacyEntity("TR phone number", "KVKK", True, "pii_scanner", "+90 / 05xx format"),
    PrivacyEntity("IBAN (TR)", "KVKK", True, "pii_scanner", ""),
    PrivacyEntity("Customer number", "KVKK", True, "custom_scanner", "Policy-defined regex"),
    PrivacyEntity("Personnel ID", "KVKK", True, "custom_scanner", "Policy-defined regex"),
    # HIPAA
    PrivacyEntity("Medical record number", "HIPAA", True, "pii_deep", "Presidio regex recognizer"),
    PrivacyEntity("Health plan beneficiary", "HIPAA", True, "pii_deep", "Presidio regex recognizer"),
    PrivacyEntity("NPI (National Provider ID)", "HIPAA", True, "pii_deep", "Presidio regex recognizer"),
    PrivacyEntity("DEA number", "HIPAA", False, "", "Not yet implemented"),
    PrivacyEntity("Medical device serial", "HIPAA", False, "", "Not yet implemented"),
    # PCI-DSS
    PrivacyEntity("Credit card number", "PCI-DSS", True, "pii_scanner + bin", "Luhn + BIN enrichment"),
    PrivacyEntity("CVV / CVC", "PCI-DSS", False, "", "Not stored per PCI requirement"),
    # Cross-regulation
    PrivacyEntity("API keys / secrets", "ALL", True, "secret_scanner", "14 key types"),
    PrivacyEntity("Connection strings", "ALL", True, "secret_scanner", "Postgres/MySQL/Mongo/Redis"),
    PrivacyEntity("JWT tokens", "ALL", True, "secret_scanner", ""),
]

