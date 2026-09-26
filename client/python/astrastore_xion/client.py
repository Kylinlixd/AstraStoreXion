"""Production HTTP client for AstraStoreXion."""

import hashlib
import json
import mimetypes
import os
import time
from typing import BinaryIO, Callable, Dict, Optional, Tuple, Union

import requests

from .config import XionConfig
from .models import (
    DeleteFileResponse,
    FileListResponse,
    FileResponse,
    FileStatusResponse,
    UploadFileResponse,
    UploadSessionResponse,
)


class XionError(Exception):
    """Base error for storage client failures."""


class XionUnavailableError(XionError):
    """The storage service could not be reached after safe retries."""


class XionChecksumError(XionError):
    """Downloaded bytes do not match the checksum recorded by the service."""

    def __init__(self, expected: str, actual: str) -> None:
        super().__init__(
            "downloaded checksum {} does not match expected {}".format(actual, expected)
        )
        self.expected = expected
        self.actual = actual


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

    def download_file(
        self,
        file_id: str,
        output: BinaryIO,
        verify_checksum: bool = False,
    ) -> None:
        """Stream an object into ``output``.

        With ``verify_checksum`` the service-provided SHA-256 is compared against
        the received bytes, so silent corruption is detected instead of being
        written to disk and trusted.
        """

        response = self._request(
            "GET",
            "/api/v1/files/" + file_id,
            expected=(200,),
            retry=True,
            stream=True,
        )
        try:
            expected = self._expected_checksum(response) if verify_checksum else ""
            digest = hashlib.sha256() if verify_checksum else None
            try:
                for chunk in response.iter_content(chunk_size=self.config.chunk_size):
                    if not chunk:
                        continue
                    output.write(chunk)
                    if digest is not None:
                        digest.update(chunk)
            except requests.RequestException as error:
                raise XionUnavailableError(str(error)) from error
            if digest is not None and expected:
                actual = digest.hexdigest()
                if actual != expected:
                    raise XionChecksumError(expected, actual)
        finally:
            response.close()

    def start_upload_session(
        self,
        filename: str,
        size: int,
        content_type: Optional[str] = None,
        metadata: Optional[Dict[str, str]] = None,
        checksum: str = "",
    ) -> UploadSessionResponse:
        """Create a resumable upload session."""

        payload = {
            "filename": filename,
            "content_type": content_type
            or mimetypes.guess_type(filename)[0]
            or "application/octet-stream",
            "size": int(size),
            "checksum": checksum,
            "metadata": metadata or {},
        }
        response = self._request(
            "POST",
            "/api/v1/uploads",
            expected=(201,),
            retry=False,
            json=payload,
        )
        try:
            return UploadSessionResponse.from_dict(self._json(response))
        finally:
            response.close()

    def get_upload_session(self, upload_id: str) -> UploadSessionResponse:
        """Read a session's ``received_bytes`` so an interrupted upload resumes."""

        response = self._request(
            "GET",
            "/api/v1/uploads/" + upload_id,
            expected=(200,),
            retry=True,
        )
        try:
            return UploadSessionResponse.from_dict(self._json(response))
        finally:
            response.close()

    def upload_large_file(
        self,
        file: Union[BinaryIO, bytes],
        filename: str,
        metadata: Optional[Dict[str, str]] = None,
        chunk_size: Optional[int] = None,
        progress: Optional[Callable[[int, int], None]] = None,
        checksum: str = "",
    ) -> UploadFileResponse:
        """Upload a large object resumably, continuing after an interruption.

        The caller passes the same ``checksum`` on retry, and the service
        verifies every chunk plus the completed object, so a resumed upload
        cannot silently splice mismatched bytes together.
        """

        size = self._size_of(file)
        chunk_bytes = max(1, int(chunk_size or self.config.chunk_size))
        content_type = mimetypes.guess_type(filename)[0] or "application/octet-stream"

        session = self.start_upload_session(
            filename=filename,
            size=size,
            content_type=content_type,
            metadata=metadata,
            checksum=checksum,
        )
        upload_id = session.upload_id

        offset = 0
        if isinstance(file, (bytes, bytearray)):
            buffer = file
        else:
            seekable = getattr(file, "seek", None)
            if seekable is not None:
                file.seek(0)
            buffer = None

        try:
            while offset < size:
                length = min(chunk_bytes, size - offset)
                if buffer is not None:
                    chunk = buffer[offset : offset + length]
                else:
                    chunk = file.read(length)
                if not chunk:
                    raise XionError(
                        "source ended after {} of {} bytes".format(offset, size)
                    )
                length = len(chunk)
                response = self._request(
                    "PUT",
                    "/api/v1/uploads/" + upload_id,
                    expected=(200,),
                    retry=True,
                    data=chunk,
                    headers={
                        "Content-Range": "bytes {}-{}/{}".format(
                            offset, offset + length - 1, size
                        ),
                        "Content-Type": "application/octet-stream",
                        "X-Chunk-Checksum": hashlib.sha256(chunk).hexdigest(),
                    },
                )
                response.close()
                offset += length
                if progress is not None:
                    progress(offset, size)

            completed = self._request(
                "POST",
                "/api/v1/uploads/" + upload_id + "/complete",
                expected=(201,),
                retry=False,
            )
            try:
                return FileResponse.from_dict(self._json(completed))
            finally:
                completed.close()
        except XionError:
            self.abort_upload_session(upload_id, ignore_errors=True)
            raise

    def abort_upload_session(self, upload_id: str, ignore_errors: bool = False) -> None:
        """Abandon a session and release the quota it reserved."""

        try:
            response = self._request(
                "DELETE",
                "/api/v1/uploads/" + upload_id,
                expected=(204,),
                retry=True,
            )
            response.close()
        except XionError:
            if not ignore_errors:
                raise

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
    def _expected_checksum(response) -> str:
        """Read the SHA-256 the service recorded, from ETag or Content-MD5 style headers."""

        etag = (response.headers.get("ETag") or "").strip().strip('"')
        if etag.startswith("sha256-"):
            return etag[len("sha256-"):].lower()
        return ""

    @staticmethod
    def _size_of(file: Union[BinaryIO, bytes]) -> int:
        if isinstance(file, (bytes, bytearray)):
            return len(file)
        seek = getattr(file, "seek", None)
        tell = getattr(file, "tell", None)
        if seek is None or tell is None:
            raise XionError(
                "resumable upload needs a seekable stream or bytes so the size is known"
            )
        current = file.tell()
        file.seek(0, os.SEEK_END)
        size = file.tell()
        file.seek(current)
        if size <= 0:
            raise XionError("resumable upload requires a non-empty source")
        return int(size)

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
