import { useQuery } from "@tanstack/react-query";
import { getImages } from "../api/images";

export function useImages(
  page: number,
  limit: number,
  search: string,
  status: string,
  pollUntil: number
) {
  return useQuery({
    queryKey: ["images", page, limit, search, status],
    queryFn: () => getImages(page, limit, search, status),
    refetchInterval: (query) => {
      // Force-poll for 30s after an upload to catch the newly saved image
      if (pollUntil > Date.now()) return 3000;
      // Ongoing poll while any image is still processing
      const images = query.state.data?.images ?? [];
      return (images ?? []).some((img) => img.status === "processing")
        ? 5000
        : false;
    },
  });
}
