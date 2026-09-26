import hashlib
import io
import unittest
from unittest.mock import Mock, patch

import requests

from astrastore_xion import (
    XionChecksumError,
    XionClient,
    XionConfig,
    XionError,
    XionHTTPError,
    XionUnavailableError,
)


FILE_PAYLOAD = {
    "file_id": "b8c21d60-e970-4df5-890b-0d2dba93a654",
    "filename": "a.txt",
    "content_type": "text/plain",
    "size": 1,
    "checksum": "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb",
    "status": "available",
    "created_at": "2026-08-09T00:00:00Z",
    "metadata": {"owner": "blog"},
}


def response(status_code, payload=None, chunks=None):
    result = Mock()
    result.status_code = status_code
    result.json.return_value = payload
    result.iter_content.return_value = chunks or []
    result.close = Mock()
    return result


class XionClientTests(unittest.TestCase):
    def make_client(self, session):
        session.headers = {}
        session.close = Mock()
        patcher = patch("astrastore_xion.client.requests.Session", return_value=session)
        self.addCleanup(patcher.stop)
        patcher.start()
        return XionClient(XionConfig(
            api_gateway="http://xion",
            service_token="secret",
            max_retries=2,
            retry_interval=0,
        ))

    def test_upload_sends_service_token_and_maps_real_fields(self):
        session = Mock()
        session.request.return_value = response(201, FILE_PAYLOAD)
        client = self.make_client(session)

        result = client.upload_file(io.BytesIO(b"a"), "a.txt", {"owner": "blog"})

        self.assertEqual("b8c21d60-e970-4df5-890b-0d2dba93a654", result.file_id)
        self.assertEqual("text/plain", result.content_type)
        self.assertEqual("Bearer secret", session.headers["Authorization"])
        args, kwargs = session.request.call_args
        self.assertEqual("POST", args[0])
        self.assertEqual("http://xion/api/v1/files", args[1])
        self.assertEqual(1, session.request.call_count)
        self.assertIn("file", kwargs["files"])

    def test_upload_is_not_retried(self):
        session = Mock()
        session.request.return_value = response(503, {
            "error": {"code": "unavailable", "message": "try later"}
        })
        client = self.make_client(session)

        with self.assertRaises(XionHTTPError) as raised:
            client.upload_file(io.BytesIO(b"a"), "a.txt")

        self.assertEqual(503, raised.exception.status_code)
        self.assertEqual("unavailable", raised.exception.code)
        self.assertEqual(1, session.request.call_count)

    def test_status_retries_transient_response(self):
        session = Mock()
        session.request.side_effect = [
            response(503, {"error": {"code": "busy", "message": "busy"}}),
            response(200, FILE_PAYLOAD),
        ]
        client = self.make_client(session)

        result = client.get_file_status(FILE_PAYLOAD["file_id"])

        self.assertEqual("available", result.status)
        self.assertEqual(2, session.request.call_count)

    def test_download_streams_chunks_and_closes_response(self):
        session = Mock()
        downloaded = response(200, chunks=[b"ab", b"", b"cd"])
        session.request.return_value = downloaded
        client = self.make_client(session)
        output = io.BytesIO()

        client.download_file(FILE_PAYLOAD["file_id"], output)

        self.assertEqual(b"abcd", output.getvalue())
        downloaded.close.assert_called_once_with()

    def test_download_wraps_interrupted_stream_and_closes_response(self):
        session = Mock()
        downloaded = response(200)
        downloaded.iter_content.side_effect = requests.ConnectionError("stream reset")
        session.request.return_value = downloaded
        client = self.make_client(session)

        with self.assertRaises(XionUnavailableError):
            client.download_file(FILE_PAYLOAD["file_id"], io.BytesIO())

        downloaded.close.assert_called_once_with()

    def test_list_and_delete_support_real_gateway_contract(self):
        session = Mock()
        session.request.side_effect = [
            response(200, {"count": 1, "results": [FILE_PAYLOAD]}),
            response(204),
        ]
        client = self.make_client(session)

        listed = client.list_files(limit=10, offset=0)
        deleted = client.delete_file(FILE_PAYLOAD["file_id"])

        self.assertEqual(1, listed.count)
        self.assertEqual(FILE_PAYLOAD["file_id"], listed.results[0].file_id)
        self.assertTrue(deleted.success)


if __name__ == "__main__":
    unittest.main()


SESSION_PAYLOAD = {
    "upload_id": "11111111-2222-3333-4444-555555555555",
    "filename": "big.bin",
    "content_type": "application/octet-stream",
    "size": 8,
    "received_bytes": 0,
    "status": "uploading",
    "created_at": "2026-09-25T00:00:00Z",
    "updated_at": "2026-09-25T00:00:00Z",
    "metadata": {"owner": "blog"},
}


class ResumableUploadTests(unittest.TestCase):
    def make_client(self, session, chunk_size=4):
        session.headers = {}
        session.close = Mock()
        patcher = patch("astrastore_xion.client.requests.Session", return_value=session)
        self.addCleanup(patcher.stop)
        patcher.start()
        return XionClient(XionConfig(
            api_gateway="http://xion",
            service_token="secret",
            max_retries=0,
            retry_interval=0,
            chunk_size=chunk_size,
        ))

    def test_upload_large_file_uses_resumable_sessions(self):
        session = Mock()
        session.request.side_effect = [
            response(201, SESSION_PAYLOAD),
            response(200, dict(SESSION_PAYLOAD, received_bytes=4)),
            response(200, dict(SESSION_PAYLOAD, received_bytes=8)),
            response(201, FILE_PAYLOAD),
        ]
        client = self.make_client(session)
        progress = []
        stored = client.upload_large_file(
            io.BytesIO(b"abcdefgh"),
            "big.bin",
            metadata={"owner": "blog"},
            progress=lambda done, total: progress.append((done, total)),
        )

        assert stored.file_id == FILE_PAYLOAD["file_id"]
        assert progress == [(4, 8), (8, 8)]

        calls = session.request.call_args_list
        assert calls[0].args[0] == "POST"
        assert calls[0].args[1].endswith("/api/v1/uploads")
        assert calls[0].kwargs["json"]["size"] == 8
        assert calls[0].kwargs["json"]["metadata"] == {"owner": "blog"}

        assert calls[1].args[1].endswith("/api/v1/uploads/" + SESSION_PAYLOAD["upload_id"])
        assert calls[1].kwargs["headers"]["Content-Range"] == "bytes 0-3/8"
        assert calls[1].kwargs["data"] == b"abcd"
        assert calls[1].kwargs["headers"]["X-Chunk-Checksum"] == hashlib.sha256(b"abcd").hexdigest()
        assert calls[2].kwargs["headers"]["Content-Range"] == "bytes 4-7/8"

        assert calls[3].args[1].endswith("/complete")

    def test_upload_large_file_aborts_session_on_failure(self):
        session = Mock()
        session.request.side_effect = [
            response(201, SESSION_PAYLOAD),
            response(409, {"error": {"code": "upload_offset_conflict", "message": "bad"}}),
            response(204),
        ]
        client = self.make_client(session)

        with self.assertRaises(XionHTTPError):
            client.upload_large_file(io.BytesIO(b"abcdefgh"), "big.bin")

        assert session.request.call_args_list[-1].args[0] == "DELETE"

    def test_upload_large_file_rejects_unseekable_source(self):
        client = self.make_client(Mock())

        class Unseekable:
            def read(self, size):
                return b""

        with self.assertRaises(XionError):
            client.upload_large_file(Unseekable(), "stream.bin")


class DownloadChecksumTests(unittest.TestCase):
    def make_client(self, session):
        session.headers = {}
        session.close = Mock()
        patcher = patch("astrastore_xion.client.requests.Session", return_value=session)
        self.addCleanup(patcher.stop)
        patcher.start()
        return XionClient(XionConfig(
            api_gateway="http://xion",
            service_token="secret",
            max_retries=0,
            retry_interval=0,
        ))

    def test_download_verifies_checksum_when_requested(self):
        body = b"hello"
        session = Mock()
        result = response(200, chunks=[body])
        result.headers = {"ETag": '"sha256-' + hashlib.sha256(body).hexdigest() + '"'}
        session.request.return_value = result
        client = self.make_client(session)

        output = io.BytesIO()
        client.download_file(FILE_PAYLOAD["file_id"], output, verify_checksum=True)
        assert output.getvalue() == body

    def test_download_rejects_mismatched_checksum(self):
        session = Mock()
        result = response(200, chunks=[b"tampered"])
        result.headers = {"ETag": '"sha256-' + hashlib.sha256(b"original").hexdigest() + '"'}
        session.request.return_value = result
        client = self.make_client(session)

        with self.assertRaises(XionChecksumError):
            client.download_file(FILE_PAYLOAD["file_id"], io.BytesIO(), verify_checksum=True)

    def test_download_without_verification_stays_lenient(self):
        session = Mock()
        result = response(200, chunks=[b"anything"])
        result.headers = {}
        session.request.return_value = result
        client = self.make_client(session)

        output = io.BytesIO()
        client.download_file(FILE_PAYLOAD["file_id"], output)
        assert output.getvalue() == b"anything"
