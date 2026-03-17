import { useState } from "react";
import type { Image } from "../types";
import { cn } from "../lib/utils";

interface ImageCardProps {
  image: Image;
  selected: boolean;
  onSelect: () => void;
  onDeleteClick: () => void;
}

export default function ImageCard({ image, selected, onSelect, onDeleteClick }: ImageCardProps) {
  const [imgError, setImgError] = useState(false);
  const thumbnail = image.compressedUrl || image.originalUrl;
  const isProcessing = image.status === "processing";

  return (
    <div
      className={cn(
        "group relative flex flex-col rounded-2xl overflow-hidden bg-white border transition-all duration-200 cursor-pointer select-none",
        selected
          ? "border-blue-500 ring-2 ring-blue-200 shadow-md"
          : "border-[#e0e0e0] hover:border-[#b0c4de] hover:shadow-lg"
      )}
    >
      {/* Thumbnail */}
      <div className="relative aspect-square bg-[#f1f3f4] overflow-hidden">
        {thumbnail && !imgError ? (
          <img
            src={thumbnail}
            alt={image.filename}
            onError={() => setImgError(true)}
            className="w-full h-full object-cover transition-transform duration-300 group-hover:scale-[1.03]"
          />
        ) : (
          <div className="w-full h-full flex flex-col items-center justify-center gap-2">
            <svg className="w-12 h-12 text-gray-300" viewBox="0 0 24 24" fill="currentColor">
              <path d="M21 19V5c0-1.1-.9-2-2-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2zM8.5 13.5l2.5 3.01L14.5 12l4.5 6H5l3.5-4.5z"/>
            </svg>
          </div>
        )}

        {/* Hover overlay with delete */}
        <div className="img-overlay absolute inset-0 bg-black/30 flex items-end justify-end p-2">
          <button
            onClick={(e) => { e.stopPropagation(); onDeleteClick(); }}
            title="Delete"
            className="w-8 h-8 rounded-full bg-white/90 hover:bg-red-50 hover:text-red-600 text-gray-600 flex items-center justify-center shadow transition-colors"
          >
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
            </svg>
          </button>
        </div>

        {/* Checkbox — visible on hover or when selected */}
        <div
          onClick={(e) => { e.stopPropagation(); onSelect(); }}
          className={cn(
            "absolute top-2 left-2 transition-opacity",
            selected ? "opacity-100" : "opacity-0 group-hover:opacity-100"
          )}
        >
          <div className={cn(
            "w-5 h-5 rounded-full border-2 flex items-center justify-center transition-colors shadow-sm",
            selected ? "bg-blue-600 border-blue-600" : "bg-white/90 border-gray-300"
          )}>
            {selected && (
              <svg className="w-3 h-3 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
              </svg>
            )}
          </div>
        </div>

        {/* Processing badge */}
        {isProcessing && (
          <div className="absolute top-2 right-2">
            <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-amber-400/90 text-amber-900 font-medium backdrop-blur-sm">
              <svg className="animate-spin h-2.5 w-2.5" fill="none" viewBox="0 0 24 24"><circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"/><path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"/></svg>
              Processing
            </span>
          </div>
        )}
      </div>

      {/* Footer */}
      <div className="flex items-center gap-2 px-3 py-2.5">
        <svg className="w-4 h-4 text-blue-400 shrink-0" viewBox="0 0 24 24" fill="currentColor">
          <path d="M21 19V5c0-1.1-.9-2-2-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2zM8.5 13.5l2.5 3.01L14.5 12l4.5 6H5l3.5-4.5z"/>
        </svg>
        <span className="text-xs text-gray-700 truncate font-medium">{image.filename}</span>
      </div>
    </div>
  );
}

export function ImageCardSkeleton() {
  return (
    <div className="rounded-2xl overflow-hidden bg-white border border-[#e0e0e0]">
      <div className="aspect-square skeleton" />
      <div className="px-3 py-2.5 flex items-center gap-2">
        <div className="w-4 h-4 skeleton rounded shrink-0" />
        <div className="h-3 skeleton rounded flex-1" />
      </div>
    </div>
  );
}
