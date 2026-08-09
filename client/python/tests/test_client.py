import io
import unittest
from unittest.mock import Mock, patch

import requests

from astrastore_xion import (
    XionClient,
    XionConfig,
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
