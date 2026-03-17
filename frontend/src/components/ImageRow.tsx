import { useState } from "react";
import type { Image } from "../types";
import { cn } from "../lib/utils";

interface ImageRowProps {
  image: Image;
  selected: boolean;
  onSelect: () => void;
  onDeleteClick: () => void;
}

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" });
}

export default function ImageRow({ image, selected, onSelect, onDeleteClick }: ImageRowProps) {
  const [imgError, setImgError] = useState(false);
  const thumbnail = image.compressedUrl || image.originalUrl;

  return (
    <div className={cn(
      "group flex items-center gap-3 px-4 py-2.5 rounded-xl transition-colors cursor-pointer",
      selected ? "bg-blue-50" : "hover:bg-[#f1f3f4]"
    )}>
      {/* Checkbox */}
      <div
        onClick={(e) => { e.stopPropagation(); onSelect(); }}
        className={cn(
          "shrink-0 transition-opacity",
          selected ? "opacity-100" : "opacity-0 group-hover:opacity-100"
        )}
      >
        <div className={cn(
          "w-5 h-5 rounded-full border-2 flex items-center justify-center transition-colors",
          selected ? "bg-blue-600 border-blue-600" : "bg-white border-gray-300"
        )}>
          {selected && (
            <svg className="w-3 h-3 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
            </svg>
          )}
        </div>
      </div>

      {/* Thumbnail */}
      <div className="w-10 h-10 rounded-lg overflow-hidden bg-[#f1f3f4] shrink-0">
        {thumbnail && !imgError
          ? <img src={thumbnail} alt={image.filename} onError={() => setImgError(true)} className="w-full h-full object-cover" />
          : <div className="w-full h-full flex items-center justify-center">
              <svg className="w-5 h-5 text-blue-400" viewBox="0 0 24 24" fill="currentColor">
                <path d="M21 19V5c0-1.1-.9-2-2-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2zM8.5 13.5l2.5 3.01L14.5 12l4.5 6H5l3.5-4.5z"/>
              </svg>
            </div>
        }
      </div>

      {/* Name */}
      <div className="flex-1 min-w-0">
        <p className="text-sm text-gray-800 truncate font-medium">{image.filename}</p>
      </div>

      {/* Status */}
      <div className="w-28 shrink-0">
        {image.status === "processing"
          ? <span className="inline-flex items-center gap-1.5 text-xs text-amber-700 font-medium">
              <svg className="animate-spin h-3 w-3" fill="none" viewBox="0 0 24 24"><circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"/><path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"/></svg>
              Processing
            </span>
          : <span className="inline-flex items-center gap-1.5 text-xs text-green-700 font-medium">
              <span className="w-1.5 h-1.5 rounded-full bg-green-500 inline-block" />
              Ready
            </span>
        }
      </div>

      {/* Date */}
      <div className="w-32 shrink-0 text-xs text-gray-500">
        {image.createdAt ? formatDate(image.createdAt) : "—"}
      </div>

      {/* Delete — visible on hover */}
      <div className="w-8 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity">
        <button
          onClick={(e) => { e.stopPropagation(); onDeleteClick(); }}
          title="Delete"
          className="w-7 h-7 rounded-full hover:bg-gray-200 text-gray-500 hover:text-red-600 flex items-center justify-center transition-colors"
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
          </svg>
        </button>
      </div>
    </div>
  );
}

export function ImageRowSkeleton() {
  return (
    <div className="flex items-center gap-3 px-4 py-2.5">
      <div className="w-5 h-5 skeleton rounded-full shrink-0" />
      <div className="w-10 h-10 skeleton rounded-lg shrink-0" />
      <div className="flex-1 h-3.5 skeleton rounded" />
      <div className="w-20 h-3 skeleton rounded" />
      <div className="w-24 h-3 skeleton rounded" />
      <div className="w-8" />
    </div>
  );
}
