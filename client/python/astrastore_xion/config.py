"""Configuration for the AstraStoreXion Python client."""

from typing import Any, Dict, Optional

import yaml


class XionConfig:
    """Connection and retry settings.

    Legacy millisecond values (``timeout`` and ``retry_interval``) remain
    accepted so existing configuration files keep working.
    """

    def __init__(
        self,
        config_dict: Optional[Dict[str, Any]] = None,
        **overrides: Any,
    ) -> None:
        values: Dict[str, Any] = dict(config_dict or {})
        values.update(overrides)

        self.api_gateway = str(
            values.get("api_gateway", "http://127.0.0.1:8081")
        ).rstrip("/")
        self.service_token = str(values.get("service_token", ""))

        legacy_timeout = float(values.get("timeout", 30000)) / 1000
        self.connect_timeout = float(values.get("connect_timeout", min(5, legacy_timeout)))
        self.read_timeout = float(values.get("read_timeout", legacy_timeout))
        self.max_retries = max(0, int(values.get("max_retries", 2)))
        self.retry_interval = max(
            0.0,
            float(values.get("retry_interval_seconds", values.get("retry_interval", 250)))
            / (1 if "retry_interval_seconds" in values else 1000),
        )
        self.chunk_size = max(1, int(values.get("chunk_size", 1024 * 1024)))

    @property
    def request_timeout(self):
        return (self.connect_timeout, self.read_timeout)


def load_config_from_yaml(file_path: str) -> XionConfig:
    """Load a client configuration and fail clearly on malformed YAML."""

    with open(file_path, "r", encoding="utf-8") as config_file:
        values = yaml.safe_load(config_file) or {}
    if not isinstance(values, dict):
        raise ValueError("Xion configuration must be a YAML mapping")
    return XionConfig(values)
