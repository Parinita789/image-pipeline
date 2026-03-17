import api from "./axios";
import axios from "axios";
import type { APIResponse, PaginatedImages } from "../types";

export async function getImages(page = 1, limit = 12, search = "", status = "") {
  const params = new URLSearchParams({ page: String(page), limit: String(limit) });
  if (search) params.set("search", search);
  if (status) params.set("status", status);
  const res = await api.get<APIResponse<PaginatedImages>>(`/images?${params.toString()}`);
  return res.data.data;
}

// ─── Presigned upload ─────────────────────────────────────────────────────────

export interface PreparedFile {
  key: string;
  uploadUrl: string;
  filename: string;
  requestId: string;
}

export async function prepareUpload(
  files: { filename: string; contentType: string; size: number }[]
): Promise<PreparedFile[]> {
  const res = await api.post<APIResponse<PreparedFile[]>>("/images/prepare", { files });
  return res.data.data ?? [];
}

// PUT directly to S3 — bypasses the API server entirely.
// Returns the same key so the caller can confirm. Retries once on 403 (expired URL).
export async function putToS3(
  uploadUrl: string,
  file: File,
  onProgress?: (percent: number) => void
): Promise<void> {
  await axios.put(uploadUrl, file, {
    headers: { "Content-Type": file.type },
    onUploadProgress: (event) => {
      if (onProgress && event.total) {
        onProgress(Math.round((event.loaded / event.total) * 100));
      }
    },
  });
}

export async function confirmUpload(
  files: { key: string; filename: string; requestId: string }[],
  idempotencyKey: string
): Promise<void> {
  await api.post(
    "/images/confirm",
    { files },
    { headers: { "X-Idempotency-Key": idempotencyKey } }
  );
}

export async function deleteImage(id: string) {
  const res = await api.delete(`/image/${id}`);
  return res.data;
}

export async function batchDeleteImages(ids: string[]) {
  const res = await api.delete<APIResponse<{ deleted: string[]; failed: string[] }>>("/images", {
    data: { ids },
  });
  return res.data.data;
}
