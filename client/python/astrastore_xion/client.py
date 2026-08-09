"""Production HTTP client for AstraStoreXion."""

import json
import mimetypes
import time
from typing import BinaryIO, Dict, Optional, Tuple, Union

import requests

from .config import XionConfig
from .models import (
    DeleteFileResponse,
    FileListResponse,
    FileResponse,
    FileStatusResponse,
    UploadFileResponse,
)


class XionError(Exception):
    """Base error for storage client failures."""


class XionUnavailableError(XionError):
    """The storage service could not be reached after safe retries."""


class XionHTTPError(XionError):
    """A structured non-success response from the storage service."""

    def __init__(self, status_code: int, code: str, message: str) -> None:
        super().__init__(message)
        self.status_code = status_code
        self.code = code
        self.message = message


class XionClient:
    """Small synchronous client intended for trusted backend services."""

    transient_statuses = frozenset({502, 503, 504})

    def __init__(self, config: Optional[XionConfig] = None) -> None:
        self.config = config or XionConfig()
        self.session = requests.Session()
        self.session.headers.update({
            "User-Agent": "AstraStoreXion-Python/1.1",
            "Accept": "application/json",
        })
        if self.config.service_token:
            self.session.headers["Authorization"] = (
                "Bearer " + self.config.service_token
            )

    def upload_file(
        self,
        file: Union[BinaryIO, bytes],
        filename: str,
        metadata: Optional[Dict[str, str]] = None,
    ) -> UploadFileResponse:
        content_type = mimetypes.guess_type(filename)[0] or "application/octet-stream"
        response = self._request(
            "POST",
            "/api/v1/files",
            expected=(201,),
            retry=False,
            files={"file": (filename, file, content_type)},
            data={"metadata": json.dumps(metadata or {}, ensure_ascii=False)},
        )
        try:
            return FileResponse.from_dict(self._json(response))
        finally:
            response.close()

    def download_file(self, file_id: str, output: BinaryIO) -> None:
        response = self._request(
            "GET",
            "/api/v1/files/" + file_id,
            expected=(200,),
            retry=True,
            stream=True,
        )
        try:
            try:
                for chunk in response.iter_content(chunk_size=self.config.chunk_size):
                    if chunk:
                        output.write(chunk)
            except requests.RequestException as error:
                raise XionUnavailableError(str(error)) from error
        finally:
            response.close()

    def delete_file(self, file_id: str) -> DeleteFileResponse:
        response = self._request(
            "DELETE",
            "/api/v1/files/" + file_id,
            expected=(204,),
            retry=True,
        )
        response.close()
        return DeleteFileResponse(success=True, message="deleted")

    def get_file_status(self, file_id: str) -> FileStatusResponse:
        response = self._request(
            "GET",
            "/api/v1/files/" + file_id + "/status",
            expected=(200,),
            retry=True,
        )
        try:
            return FileResponse.from_dict(self._json(response))
        finally:
            response.close()

    def list_files(self, limit: int = 100, offset: int = 0) -> FileListResponse:
        response = self._request(
            "GET",
            "/api/v1/files",
            expected=(200,),
            retry=True,
            params={"limit": limit, "offset": offset},
        )
        try:
            return FileListResponse.from_dict(self._json(response))
        finally:
            response.close()

    def health(self) -> str:
        response = self._request(
            "GET", "/health", expected=(200,), retry=True
        )
        try:
            return str(self._json(response).get("status", ""))
        finally:
            response.close()

    def close(self) -> None:
        self.session.close()

    def _request(
        self,
        method: str,
        path: str,
        *,
        expected: Tuple[int, ...],
        retry: bool,
        **kwargs,
    ):
        attempts = self.config.max_retries + 1 if retry else 1
        last_exception = None
        for attempt in range(attempts):
            try:
                response = self.session.request(
                    method,
                    self.config.api_gateway + path,
                    timeout=self.config.request_timeout,
                    **kwargs,
                )
            except requests.RequestException as error:
                last_exception = error
                if attempt + 1 >= attempts:
                    raise XionUnavailableError(str(error)) from error
                self._wait(attempt)
                continue

            if response.status_code in expected:
                return response
            if retry and response.status_code in self.transient_statuses and attempt + 1 < attempts:
                response.close()
                self._wait(attempt)
                continue
            error = self._http_error(response)
            response.close()
            raise error

        raise XionUnavailableError(str(last_exception or "request failed"))

    def _wait(self, attempt: int) -> None:
        time.sleep(self.config.retry_interval * (attempt + 1))

    @staticmethod
    def _json(response) -> Dict:
        try:
            payload = response.json()
        except (ValueError, json.JSONDecodeError) as error:
            raise XionError("storage service returned invalid JSON") from error
        if not isinstance(payload, dict):
            raise XionError("storage service returned a non-object JSON response")
        return payload

    @classmethod
    def _http_error(cls, response) -> XionHTTPError:
        code = "http_error"
        message = "storage request failed with HTTP {}".format(response.status_code)
        try:
            payload = response.json()
            detail = payload.get("error", {}) if isinstance(payload, dict) else {}
            if isinstance(detail, dict):
                code = str(detail.get("code", code))
                message = str(detail.get("message", message))
        except (ValueError, json.JSONDecodeError):
            pass
        return XionHTTPError(response.status_code, code, message)

    def __enter__(self) -> "XionClient":
        return self

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        self.close()
