import { useState } from "react";
import { useImages } from "../hooks/useImages";
import { useDeleteImages } from "../hooks/useDeleteImage";
import ImageCard, { ImageCardSkeleton } from "../components/ImageCard";
import ImageRow, { ImageRowSkeleton } from "../components/ImageRow";
import Navbar from "../components/Navbar";
import Sidebar from "../components/Sidebar";
import UploadModal from "../components/UploadModal";
import DeleteConfirmModal from "../components/DeleteConfirmModal";
import { cn } from "../lib/utils";

type PendingDelete = { id: string; name: string }[];

type ViewMode = "grid" | "list";
type SidebarView = "my-drive" | "recent";

export default function Dashboard() {
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [showUpload, setShowUpload] = useState(false);
  const [pollUntil, setPollUntil] = useState(0);
  const [viewMode, setViewMode] = useState<ViewMode>("grid");
  const [sidebarView, setSidebarView] = useState<SidebarView>("my-drive");
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [pendingDelete, setPendingDelete] = useState<PendingDelete | null>(null);
  const { mutate: deleteImages, isPending: isDeleting } = useDeleteImages();
  const limit = 20;

  // "Recent" = no filter, sorted by date (backend default)
  const effectiveStatus = sidebarView === "recent" ? "" : statusFilter;
  const { data, isLoading, isError, isFetching } = useImages(page, limit, search, effectiveStatus, pollUntil);

  const images = data?.images ?? [];
  const totalPages = data ? Math.ceil(data.total / limit) : 1;

  function handleSearch(v: string) {
    setSearch(v);
    setPage(1);
  }

  function handleUploadSuccess() {
    setPollUntil(Date.now() + 30_000);
  }

  function handleSidebarView(v: SidebarView) {
    setSidebarView(v);
    setPage(1);
    setSearch("");
    setStatusFilter("");
    setSelectedIds(new Set());
  }

  function toggleSelect(id: string) {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  }

  function selectAll() {
    setSelectedIds(new Set(images.map((img) => img.id)));
  }

  function clearSelection() {
    setSelectedIds(new Set());
  }

  function openDeleteModal(items: PendingDelete) {
    setPendingDelete(items);
  }

  function handleConfirmDelete() {
    if (!pendingDelete) return;
    const ids = pendingDelete.map((i) => i.id);
    deleteImages(ids, {
      onSuccess: () => {
        setPendingDelete(null);
        setSelectedIds((prev) => {
          const next = new Set(prev);
          ids.forEach((id) => next.delete(id));
          return next;
        });
      },
    });
  }

  return (
    <div className="h-screen flex flex-col bg-[#f8f9fa] overflow-hidden">
      <Navbar searchValue={search} onSearch={handleSearch} />

      <div className="flex flex-1 overflow-hidden">
        <Sidebar
          activeView={sidebarView}
          onViewChange={handleSidebarView}
          onUploadClick={() => setShowUpload(true)}
          imageCount={data?.total ?? 0}
        />

        {/* Main content */}
        <main className="flex-1 overflow-y-auto px-6 py-4">

          {/* Selection toolbar — shown when items are selected */}
          {selectedIds.size > 0 && (
            <div className="flex items-center gap-3 mb-4 px-4 py-2.5 bg-[#e8f0fe] rounded-xl">
              <button onClick={clearSelection} className="text-gray-500 hover:text-gray-700 transition-colors">
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
              <span className="text-sm font-medium text-[#1a73e8]">
                {selectedIds.size} selected
              </span>
              <div className="flex items-center gap-2 ml-auto">
                {selectedIds.size < images.length && (
                  <button
                    onClick={selectAll}
                    className="text-xs text-[#1a73e8] hover:underline font-medium"
                  >
                    Select all {images.length}
                  </button>
                )}
                <button
                  onClick={() => openDeleteModal(
                    images
                      .filter((img) => selectedIds.has(img.id))
                      .map((img) => ({ id: img.id, name: img.filename }))
                  )}
                  className="flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-white bg-red-500 hover:bg-red-600 rounded-lg transition-colors"
                >
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
                  </svg>
                  Delete
                </button>
              </div>
            </div>
          )}

          {/* Toolbar */}
          <div className="flex items-center justify-between mb-4">
            {/* Breadcrumb */}
            <div className="flex items-center gap-1.5 text-sm text-gray-700">
              <span className="font-medium">{sidebarView === "recent" ? "Recent" : "My Drive"}</span>
              {search && (
                <>
                  <svg className="w-4 h-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                  </svg>
                  <span className="text-gray-500">Search results</span>
                </>
              )}
            </div>

            <div className="flex items-center gap-2">
              {/* Status filter — only on My Drive */}
              {sidebarView === "my-drive" && (
                <div className="flex items-center gap-1 mr-2">
                  {([["", "All"], ["processing", "Processing"], ["compressed", "Ready"]] as const).map(([val, label]) => (
                    <button key={val} onClick={() => { setStatusFilter(val); setPage(1); }}
                      className={cn(
                        "px-3 py-1.5 rounded-full text-xs font-medium transition-colors",
                        statusFilter === val
                          ? "bg-[#c2e7ff] text-[#001d35]"
                          : "text-gray-600 hover:bg-gray-200"
                      )}>
                      {label}
                    </button>
                  ))}
                </div>
              )}

              {/* Syncing indicator */}
              {isFetching && !isLoading && (
                <svg className="animate-spin w-4 h-4 text-blue-500" fill="none" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"/>
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"/>
                </svg>
              )}

              {/* View toggle */}
              <div className="flex items-center bg-white border border-gray-200 rounded-lg overflow-hidden">
                <button onClick={() => setViewMode("grid")}
                  className={cn("p-2 transition-colors", viewMode === "grid" ? "bg-[#c2e7ff] text-[#001d35]" : "text-gray-500 hover:bg-gray-100")}>
                  <svg className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M3 3h7v7H3zm0 11h7v7H3zm11-11h7v7h-7zm0 11h7v7h-7z"/>
                  </svg>
                </button>
                <button onClick={() => setViewMode("list")}
                  className={cn("p-2 transition-colors", viewMode === "list" ? "bg-[#c2e7ff] text-[#001d35]" : "text-gray-500 hover:bg-gray-100")}>
                  <svg className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M3 13h2v-2H3v2zm0 4h2v-2H3v2zm0-8h2V7H3v2zm4 4h14v-2H7v2zm0 4h14v-2H7v2zM7 7v2h14V7H7z"/>
                  </svg>
                </button>
              </div>
            </div>
          </div>

          {/* Error */}
          {isError && (
            <div className="flex flex-col items-center justify-center py-24 text-center">
              <svg className="w-12 h-12 text-gray-300 mb-3" fill="currentColor" viewBox="0 0 24 24">
                <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z"/>
              </svg>
              <p className="text-sm text-gray-600 font-medium">Something went wrong</p>
              <p className="text-xs text-gray-400 mt-1">Check your connection and try again</p>
            </div>
          )}

          {/* List view header */}
          {!isLoading && !isError && images.length > 0 && viewMode === "list" && (
            <div className="flex items-center gap-3 px-4 py-1.5 text-xs text-gray-500 font-medium uppercase tracking-wide border-b border-gray-200 mb-1">
              <div className="w-5 shrink-0" />
              <div className="w-10 shrink-0" />
              <div className="flex-1">Name</div>
              <div className="w-28 shrink-0">Status</div>
              <div className="w-32 shrink-0">Date</div>
              <div className="w-8 shrink-0" />
            </div>
          )}

          {/* Skeletons */}
          {isLoading && viewMode === "grid" && (
            <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
              {Array.from({ length: 10 }).map((_, i) => <ImageCardSkeleton key={i} />)}
            </div>
          )}
          {isLoading && viewMode === "list" && (
            <div className="space-y-1">{Array.from({ length: 8 }).map((_, i) => <ImageRowSkeleton key={i} />)}</div>
          )}

          {/* Empty state */}
          {!isLoading && !isError && images.length === 0 && (
            <div className="flex flex-col items-center justify-center py-24 text-center">
              <svg className="w-20 h-20 text-gray-200 mb-4" viewBox="0 0 24 24" fill="currentColor">
                <path d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-7 3c1.93 0 3.5 1.57 3.5 3.5S13.93 13 12 13s-3.5-1.57-3.5-3.5S10.07 6 12 6zm7 13H5v-.23c0-.62.28-1.2.76-1.58C7.47 15.82 9.64 15 12 15s4.53.82 6.24 2.19c.48.38.76.97.76 1.58V19z"/>
              </svg>
              <p className="text-base font-medium text-gray-600">
                {search ? `No results for "${search}"` : "No images here"}
              </p>
              <p className="text-sm text-gray-400 mt-1">
                {search ? "Try a different search term" : "Click New to upload your first image"}
              </p>
            </div>
          )}

          {/* Grid view */}
          {!isLoading && viewMode === "grid" && images.length > 0 && (
            <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
              {images.map((image) => (
                <ImageCard
                  key={image.id}
                  image={image}
                  selected={selectedIds.has(image.id)}
                  onSelect={() => toggleSelect(image.id)}
                  onDeleteClick={() => openDeleteModal([{ id: image.id, name: image.filename }])}
                />
              ))}
            </div>
          )}

          {/* List view */}
          {!isLoading && viewMode === "list" && images.length > 0 && (
            <div className="space-y-0.5">
              {images.map((image) => (
                <ImageRow
                  key={image.id}
                  image={image}
                  selected={selectedIds.has(image.id)}
                  onSelect={() => toggleSelect(image.id)}
                  onDeleteClick={() => openDeleteModal([{ id: image.id, name: image.filename }])}
                />
              ))}
            </div>
          )}

          {/* Pagination */}
          {!isLoading && totalPages > 1 && (
            <div className="flex items-center justify-center gap-1 mt-8 mb-4">
              <button onClick={() => setPage((p) => Math.max(1, p - 1))} disabled={page === 1}
                className="w-9 h-9 rounded-full flex items-center justify-center text-gray-600 hover:bg-gray-200 disabled:opacity-30 transition-colors">
                <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M15 19l-7-7 7-7" />
                </svg>
              </button>
              {Array.from({ length: totalPages }, (_, i) => i + 1)
                .filter((p) => p === 1 || p === totalPages || Math.abs(p - page) <= 1)
                .reduce<(number | "…")[]>((acc, p, idx, arr) => {
                  if (idx > 0 && (p as number) - (arr[idx - 1] as number) > 1) acc.push("…");
                  acc.push(p);
                  return acc;
                }, [])
                .map((p, i) =>
                  p === "…"
                    ? <span key={`dots-${i}`} className="w-9 text-center text-sm text-gray-400">…</span>
                    : <button key={p} onClick={() => setPage(p as number)}
                        className={cn("w-9 h-9 rounded-full text-sm font-medium transition-colors",
                          page === p ? "bg-[#c2e7ff] text-[#001d35]" : "text-gray-600 hover:bg-gray-200")}>
                        {p}
                      </button>
                )}
              <button onClick={() => setPage((p) => Math.min(totalPages, p + 1))} disabled={page === totalPages}
                className="w-9 h-9 rounded-full flex items-center justify-center text-gray-600 hover:bg-gray-200 disabled:opacity-30 transition-colors">
                <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                </svg>
              </button>
            </div>
          )}
        </main>
      </div>

      {showUpload && (
        <UploadModal onClose={() => setShowUpload(false)} onUploadSuccess={handleUploadSuccess} />
      )}

      {pendingDelete && (
        <DeleteConfirmModal
          images={pendingDelete}
          onConfirm={handleConfirmDelete}
          onCancel={() => setPendingDelete(null)}
          isDeleting={isDeleting}
        />
      )}
    </div>
  );
}
