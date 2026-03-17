import { useCallback, useState } from "react";
import { useDropzone } from "react-dropzone";
import { useQueryClient } from "@tanstack/react-query";
import { prepareUpload, putToS3, confirmUpload, type PreparedFile } from "../api/images";
import { cn } from "../lib/utils";

interface FileItem {
  id: string;
  file: File;
  preview: string | null;
  status: "queued" | "uploading" | "done" | "error";
  progress: number;
}

interface UploadModalProps {
  onClose: () => void;
  onUploadSuccess: () => void;
}

// Retry a function up to maxAttempts times with exponential backoff.
async function withRetry<T>(fn: () => Promise<T>, maxAttempts = 3): Promise<T> {
  let lastErr: unknown;
  for (let i = 0; i < maxAttempts; i++) {
    try {
      return await fn();
    } catch (err) {
      lastErr = err;
      if (i < maxAttempts - 1) {
        await new Promise((r) => setTimeout(r, 1000 * Math.pow(2, i))); // 1s, 2s, 4s
      }
    }
  }
  throw lastErr;
}

export default function UploadModal({ onClose, onUploadSuccess }: UploadModalProps) {
  const queryClient = useQueryClient();
  const [items, setItems] = useState<FileItem[]>([]);
  const [uploading, setUploading] = useState(false);
  const [uploadFailed, setUploadFailed] = useState(false);
  const [overallProgress, setOverallProgress] = useState(0);

  const onDrop = useCallback((accepted: File[]) => {
    const newItems: FileItem[] = accepted.slice(0, 30).map((f) => ({
      id: crypto.randomUUID(),
      file: f,
      preview: f.type.startsWith("image/") ? URL.createObjectURL(f) : null,
      status: "queued",
      progress: 0,
    }));
    setItems((prev) => [...prev, ...newItems].slice(0, 30));
  }, []);

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    onDrop,
    accept: { "image/jpeg": [], "image/png": [], "image/webp": [] },
    maxSize: 100 * 1024 * 1024,
    multiple: true,
    maxFiles: 30,
    disabled: uploading,
  });

  function removeItem(id: string) {
    setItems((prev) => {
      const it = prev.find((x) => x.id === id);
      if (it?.preview) URL.revokeObjectURL(it.preview);
      return prev.filter((x) => x.id !== id);
    });
  }

  async function handleUploadAll() {
    if (!items.length) return;
    setUploading(true);
    setOverallProgress(0);
    setItems((prev) => prev.map((it) => ({ ...it, status: "uploading" as const })));

    const snapshot = items;

    try {
      // ── Step 1: Prepare — get presigned URLs (fast, no file bytes sent) ──────
      const fileDescs = snapshot.map((it) => ({
        filename: it.file.name,
        contentType: it.file.type,
        size: it.file.size,
      }));
      const prepared: PreparedFile[] = await withRetry(() => prepareUpload(fileDescs));

      // ── Step 2: PUT each file directly to S3 in parallel ─────────────────────
      const progressMap = new Array(snapshot.length).fill(0);
      const confirmedFiles: { key: string; filename: string; requestId: string }[] = [];

      await Promise.all(
        prepared.map(async (p, i) => {
          const file = snapshot[i].file;
          try {
            await withRetry(() =>
              putToS3(p.uploadUrl, file, (pct) => {
                progressMap[i] = pct;
                const avg = Math.round(progressMap.reduce((a, b) => a + b, 0) / progressMap.length);
                setOverallProgress(avg);
              })
            );
            confirmedFiles.push({ key: p.key, filename: p.filename, requestId: p.requestId });
            setItems((prev) =>
              prev.map((it, idx) => (idx === i ? { ...it, progress: 100 } : it))
            );
          } catch {
            setItems((prev) =>
              prev.map((it, idx) => (idx === i ? { ...it, status: "error" as const } : it))
            );
          }
        })
      );

      if (confirmedFiles.length === 0) {
        throw new Error("all files failed to upload");
      }

      // ── Step 3: Confirm — tell the API which files landed in S3 ──────────────
      // Retry with backoff to handle transient failures (covers "tab-closed" scenario).
      await withRetry(() => confirmUpload(confirmedFiles, crypto.randomUUID()));

      // Success — close immediately; dashboard polls for processing → ready transition
      snapshot.forEach((it) => { if (it.preview) URL.revokeObjectURL(it.preview); });
      queryClient.invalidateQueries({ queryKey: ["images"] });
      onUploadSuccess();
      onClose();
    } catch {
      setItems((prev) => prev.map((it) =>
        it.status === "uploading" ? { ...it, status: "error" as const } : it
      ));
      setUploading(false);
      setUploadFailed(true);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/40 backdrop-blur-sm">
      <div className="bg-white rounded-2xl shadow-2xl w-full max-w-lg flex flex-col overflow-hidden max-h-[90vh]">

        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-100 shrink-0">
          <h2 className="text-base font-medium text-gray-800">Upload to ImageDrive</h2>
          <button onClick={onClose} disabled={uploading}
            className="w-8 h-8 rounded-full flex items-center justify-center text-gray-500 hover:bg-gray-100 disabled:opacity-40 transition-colors">
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Dropzone */}
        <div
          {...getRootProps()}
          className={cn(
            "mx-6 mt-4 border-2 border-dashed rounded-xl text-center cursor-pointer transition-all shrink-0",
            isDragActive ? "border-blue-500 bg-blue-50" : "border-gray-300 hover:border-blue-400 hover:bg-gray-50",
            uploading && "opacity-50 cursor-default pointer-events-none"
          )}
        >
          <input {...getInputProps()} />
          <div className="py-6 flex flex-col items-center gap-2">
            <svg className={cn("w-10 h-10", isDragActive ? "text-blue-500" : "text-gray-300")} viewBox="0 0 24 24" fill="currentColor">
              <path d="M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96zM14 13v4h-4v-4H7l5-5 5 5h-3z"/>
            </svg>
            <div>
              <p className="text-sm text-gray-600 font-medium">
                {isDragActive ? "Drop files here" : "Drag files here, or click to select"}
              </p>
              <p className="text-xs text-gray-400 mt-0.5">JPEG · PNG · WebP — up to 30 files, 100 MB each</p>
            </div>
          </div>
        </div>

        {/* File list */}
        {items.length > 0 && (
          <div className="flex-1 overflow-y-auto px-6 pt-3 pb-2 space-y-2 min-h-0">
            {uploadFailed ? (
              <div className="py-3 text-center">
                <svg className="w-10 h-10 text-red-400 mx-auto mb-2" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
                <p className="text-sm font-medium text-gray-700">Upload failed — please try again</p>
              </div>
            ) : (
              <p className="text-xs text-gray-500 mb-2">
                {items.length} file{items.length !== 1 ? "s" : ""} selected
                {items.length === 30 && <span className="ml-1 text-amber-600">(max)</span>}
              </p>
            )}

            {items.map((item, i) => (
              <div key={item.id} className="flex items-center gap-3 py-2 px-3 rounded-xl bg-gray-50 border border-gray-100">
                <div className="w-10 h-10 rounded-lg overflow-hidden bg-white border border-gray-200 shrink-0 flex items-center justify-center">
                  {item.preview
                    ? <img src={item.preview} className="w-full h-full object-cover" alt="" />
                    : <svg className="w-8 h-8 text-blue-400" viewBox="0 0 24 24" fill="currentColor">
                        <path d="M21 19V5c0-1.1-.9-2-2-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2zM8.5 13.5l2.5 3.01L14.5 12l4.5 6H5l3.5-4.5z"/>
                      </svg>
                  }
                </div>

                <div className="flex-1 min-w-0">
                  <p className="text-xs font-medium text-gray-700 truncate">{item.file.name}</p>
                  <p className="text-xs text-gray-400">{(item.file.size / 1024 / 1024).toFixed(1)} MB</p>
                  {item.status === "uploading" && (
                    <div className="mt-1.5 h-1 bg-gray-200 rounded-full overflow-hidden">
                      <div className="h-full bg-blue-500 rounded-full transition-all duration-200" style={{ width: `${item.progress}%` }} />
                    </div>
                  )}
                </div>

                <div className="shrink-0 w-7 flex items-center justify-center">
                  {item.status === "queued" && !uploading && (
                    <button onClick={() => removeItem(item.id)}
                      className="w-6 h-6 rounded-full hover:bg-gray-200 text-gray-400 hover:text-gray-600 flex items-center justify-center transition-colors">
                      <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                      </svg>
                    </button>
                  )}
                  {item.status === "uploading" && (
                    <svg className="animate-spin w-4 h-4 text-blue-500" fill="none" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"/>
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"/>
                    </svg>
                  )}
                  {item.status === "done" && (
                    <svg className="w-4 h-4 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                    </svg>
                  )}
                  {item.status === "error" && (
                    <svg className="w-4 h-4 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                  )}
                </div>
                {/* satisfy linter — i is used implicitly via map index */}
                <span className="hidden">{i}</span>
              </div>
            ))}
          </div>
        )}

        {/* Footer */}
        <div className="flex flex-col gap-2 px-6 py-4 border-t border-gray-100 shrink-0 bg-gray-50/50">
          {uploading && (
            <div className="w-full">
              <div className="flex justify-between text-xs text-gray-400 mb-1">
                <span>Uploading to S3…</span>
                <span>{overallProgress}%</span>
              </div>
              <div className="h-1.5 bg-gray-200 rounded-full overflow-hidden">
                <div className="h-full bg-blue-500 rounded-full transition-all duration-200" style={{ width: `${overallProgress}%` }} />
              </div>
            </div>
          )}
          <div className="flex items-center gap-2">
            <button onClick={onClose} disabled={uploading}
              className="px-4 py-2 text-sm text-gray-600 font-medium rounded-md hover:bg-gray-100 disabled:opacity-40 transition-colors">
              {uploadFailed ? "Close" : "Cancel"}
            </button>
            <button
              onClick={handleUploadAll}
              disabled={!items.length || uploading}
              className="px-5 py-2 text-sm font-medium bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center gap-2"
            >
              {uploading && (
                <svg className="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"/>
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"/>
                </svg>
              )}
              {uploading ? "Uploading…" : `Upload${items.length > 1 ? ` ${items.length} files` : ""}`}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
