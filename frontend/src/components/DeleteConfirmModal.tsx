interface Props {
  images: { id: string; name: string }[];
  onConfirm: () => void;
  onCancel: () => void;
  isDeleting: boolean;
}

export default function DeleteConfirmModal({ images, onConfirm, onCancel, isDeleting }: Props) {
  const count = images.length;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/40 backdrop-blur-sm">
      <div className="bg-white rounded-2xl shadow-2xl w-full max-w-sm overflow-hidden">
        {/* Icon + text */}
        <div className="flex flex-col items-center px-6 pt-7 pb-5 text-center">
          <div className="w-12 h-12 rounded-full bg-red-50 flex items-center justify-center mb-4">
            <svg className="w-6 h-6 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
          </div>

          <h3 className="text-base font-semibold text-gray-800 mb-1">
            {count === 1 ? "Delete image?" : `Delete ${count} images?`}
          </h3>

          <p className="text-sm text-gray-500">
            {count === 1 ? (
              <><span className="font-medium text-gray-700">"{images[0].name}"</span> will be permanently deleted.</>
            ) : (
              `${count} selected images will be permanently deleted.`
            )}
          </p>

          {count > 1 && count <= 6 && (
            <ul className="mt-2 w-full text-xs text-gray-400 space-y-0.5 max-h-32 overflow-y-auto">
              {images.map((img) => (
                <li key={img.id} className="truncate px-2">{img.name}</li>
              ))}
            </ul>
          )}

          <p className="text-xs text-gray-400 mt-3">This action cannot be undone.</p>
        </div>

        {/* Actions */}
        <div className="flex gap-3 px-6 pb-6">
          <button
            onClick={onCancel}
            disabled={isDeleting}
            className="flex-1 py-2.5 text-sm font-medium text-gray-700 bg-gray-100 hover:bg-gray-200 rounded-lg disabled:opacity-50 transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            disabled={isDeleting}
            className="flex-1 py-2.5 text-sm font-medium text-white bg-red-500 hover:bg-red-600 rounded-lg disabled:opacity-50 transition-colors flex items-center justify-center gap-2"
          >
            {isDeleting && (
              <svg className="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"/>
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"/>
              </svg>
            )}
            Delete
          </button>
        </div>
      </div>
    </div>
  );
}
