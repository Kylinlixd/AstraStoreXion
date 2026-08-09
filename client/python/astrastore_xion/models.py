"""Response models returned by AstraStoreXion."""

from dataclasses import dataclass, field
from typing import Dict, List


@dataclass(frozen=True)
class FileResponse:
    file_id: str
    filename: str
    content_type: str
    size: int
    checksum: str
    status: str
    created_at: str
    metadata: Dict[str, str] = field(default_factory=dict)

    @classmethod
    def from_dict(cls, data: Dict) -> "FileResponse":
        return cls(
            file_id=str(data.get("file_id", "")),
            filename=str(data.get("filename", "")),
            content_type=str(data.get("content_type", "application/octet-stream")),
            size=int(data.get("size", 0)),
            checksum=str(data.get("checksum", "")),
            status=str(data.get("status", "available")),
            created_at=str(data.get("created_at", "")),
            metadata=dict(data.get("metadata") or {}),
        )


UploadFileResponse = FileResponse
FileStatusResponse = FileResponse


@dataclass(frozen=True)
class FileListResponse:
    count: int
    results: List[FileResponse]

    @classmethod
    def from_dict(cls, data: Dict) -> "FileListResponse":
        results = [FileResponse.from_dict(item) for item in data.get("results", [])]
        return cls(count=int(data.get("count", len(results))), results=results)


@dataclass(frozen=True)
class DeleteFileResponse:
    success: bool
    message: str = ""
