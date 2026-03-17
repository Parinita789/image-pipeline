import { useAuthStore } from "../store/authStore";
import { useNavigate } from "react-router-dom";

interface NavbarProps {
  searchValue: string;
  onSearch: (v: string) => void;
}

export default function Navbar({ searchValue, onSearch }: NavbarProps) {
  const logout = useAuthStore((s) => s.logout);
  const firstName = useAuthStore((s) => s.firstName);
  const navigate = useNavigate();
  const initial = (firstName ?? "U")[0].toUpperCase();

  return (
    <header className="h-16 flex items-center gap-3 px-4 bg-[#f8f9fa] border-b border-[#e0e0e0] shrink-0">
      {/* Logo */}
      <div className="flex items-center gap-2 w-56 shrink-0">
        <svg className="w-8 h-8" viewBox="0 0 87.3 78" fill="none" xmlns="http://www.w3.org/2000/svg">
          <path d="M6.6 66.85L23.4 45.8 0 45.8z" fill="#0066DA"/>
          <path d="M43.65 0L20.25 40.4h46.8z" fill="#00AC47"/>
          <path d="M59.85 45.8L76.65 66.85 87.3 45.8z" fill="#EA4335"/>
          <path d="M43.65 0L6.6 66.85h37.05L80.7 0z" fill="#00832D"/>
          <path d="M80.7 0L43.65 0 76.65 66.85h10.65z" fill="#2684FC"/>
          <path d="M6.6 66.85h73.05L76.65 66.85H6.6z" fill="#FFBA00"/>
        </svg>
        <span className="text-lg text-gray-700 font-medium tracking-tight" style={{ fontFamily: "'Google Sans', sans-serif" }}>
          ImageDrive
        </span>
      </div>

      {/* Search bar */}
      <div className="flex-1 max-w-2xl">
        <div className="relative">
          <svg className="absolute left-4 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            type="text"
            placeholder="Search in ImageDrive"
            value={searchValue}
            onChange={(e) => onSearch(e.target.value)}
            className="w-full pl-12 pr-4 py-2.5 bg-[#eaf1fb] hover:bg-[#e3eaf3] focus:bg-white rounded-full text-sm text-gray-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:shadow-md transition-all placeholder:text-gray-500"
          />
          {searchValue && (
            <button
              onClick={() => onSearch("")}
              className="absolute right-4 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-700"
            >
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          )}
        </div>
      </div>

      {/* User avatar */}
      <div className="ml-auto flex items-center gap-2">
        <button
          onClick={() => { logout(); navigate("/login"); }}
          className="flex items-center gap-2 px-3 py-1.5 rounded-full text-sm text-gray-600 hover:bg-gray-200 transition-colors"
          title="Sign out"
        >
          <div className="w-8 h-8 rounded-full bg-blue-600 flex items-center justify-center text-white text-sm font-medium select-none">
            {initial}
          </div>
          <svg className="w-4 h-4 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
          </svg>
        </button>
      </div>
    </header>
  );
}
