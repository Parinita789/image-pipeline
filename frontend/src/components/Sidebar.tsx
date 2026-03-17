import { cn } from "../lib/utils";

type SidebarView = "my-drive" | "recent";

interface SidebarProps {
  activeView: SidebarView;
  onViewChange: (v: SidebarView) => void;
  onUploadClick: () => void;
  imageCount: number;
}

const NAV_ITEMS: { id: SidebarView; label: string; icon: React.ReactNode }[] = [
  {
    id: "my-drive",
    label: "My Drive",
    icon: (
      <svg className="w-5 h-5" viewBox="0 0 24 24" fill="currentColor">
        <path d="M19 2H5c-1.1 0-2 .9-2 2v16c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm-7 3c1.93 0 3.5 1.57 3.5 3.5S13.93 12 12 12s-3.5-1.57-3.5-3.5S10.07 5 12 5zm7 13H5v-.23c0-.62.28-1.2.76-1.58C7.47 14.82 9.64 14 12 14s4.53.82 6.24 2.19c.48.38.76.97.76 1.58V18z" opacity={0} />
        <path d="M20 6h-2.18c.07-.31.18-.61.18-.93C18 3.16 16.84 2 15.07 2c-.98 0-1.95.55-2.45 1.38L12 4.5l-.62-1.12C10.88 2.55 9.91 2 8.93 2 7.16 2 6 3.16 6 4.07c0 .32.11.62.18.93H4c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2zm-5-2c.55 0 1 .45 1 1s-.45 1-1 1-1-.45-1-1 .45-1 1-1zM9 4.07c0-.55.45-1 1-1s1 .45 1 1-.45 1-1 1-1-.45-1-1zM20 20H4v-2l4-3.99 2.5 2.5 3.5-4.51L18 16v-.01h2V20z"/>
      </svg>
    ),
  },
  {
    id: "recent",
    label: "Recent",
    icon: (
      <svg className="w-5 h-5" viewBox="0 0 24 24" fill="currentColor">
        <path d="M13 3c-4.97 0-9 4.03-9 9H1l3.89 3.89.07.14L9 12H6c0-3.87 3.13-7 7-7s7 3.13 7 7-3.13 7-7 7c-1.93 0-3.68-.79-4.94-2.06l-1.42 1.42C8.27 19.99 10.51 21 13 21c4.97 0 9-4.03 9-9s-4.03-9-9-9zm-1 5v5l4.28 2.54.72-1.21-3.5-2.08V8H12z"/>
      </svg>
    ),
  },
];

export default function Sidebar({ activeView, onViewChange, onUploadClick, imageCount }: SidebarProps) {
  return (
    <aside className="w-60 shrink-0 flex flex-col py-3 bg-[#f8f9fa] overflow-y-auto">
      {/* New button */}
      <div className="px-3 mb-3">
        <button
          onClick={onUploadClick}
          className="flex items-center gap-3 pl-4 pr-6 py-3.5 bg-white hover:bg-gray-50 border border-gray-200 rounded-2xl shadow-sm hover:shadow-md text-sm font-medium text-gray-700 transition-all w-full"
        >
          <svg className="w-6 h-6 text-gray-600" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
          </svg>
          New
        </button>
      </div>

      {/* Nav items */}
      <nav className="flex-1 px-2">
        {NAV_ITEMS.map((item) => (
          <button
            key={item.id}
            onClick={() => onViewChange(item.id)}
            className={cn(
              "w-full flex items-center gap-4 px-4 py-2 rounded-r-full text-sm transition-colors text-left",
              activeView === item.id
                ? "bg-[#c2e7ff] text-[#001d35] font-medium"
                : "text-gray-700 hover:bg-gray-200"
            )}
          >
            <span className={activeView === item.id ? "text-[#001d35]" : "text-gray-600"}>
              {item.icon}
            </span>
            {item.label}
          </button>
        ))}
      </nav>

      {/* Storage */}
      <div className="px-5 py-4 border-t border-gray-200 mt-2">
        <div className="flex items-center gap-2 mb-2">
          <svg className="w-4 h-4 text-gray-500" fill="currentColor" viewBox="0 0 24 24">
            <path d="M12 3C6.48 3 2 7.48 2 13s4.48 10 10 10 10-4.48 10-10S17.52 3 12 3zm1 17.93V19h-2v1.93C7.06 20.48 4 17.24 4 13c0-4.07 2.95-7.45 6.84-8.07-.07.36-.12.73-.15 1.07H9v2h1.69C10.27 8.52 10 9.23 10 10h2c0-.55.45-1 1-1s1 .45 1 1v1h2v-1c0-1.3-.84-2.4-2-2.82V5.07C17.05 5.59 20 8.93 20 13c0 4.24-3.06 7.48-7 7.93z" opacity={0}/>
            <path d="M18 2h-8L4 8v12c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm-5 15H8v-2h5v2zm3-4H8v-2h8v2zm0-4H8V7h8v2z"/>
          </svg>
          <span className="text-xs text-gray-600 font-medium">Storage</span>
        </div>
        <p className="text-xs text-gray-500">{imageCount} {imageCount === 1 ? "image" : "images"} stored</p>
        <div className="mt-2 h-1.5 bg-gray-200 rounded-full overflow-hidden">
          <div className="h-full w-1/3 bg-blue-500 rounded-full" />
        </div>
      </div>
    </aside>
  );
}
