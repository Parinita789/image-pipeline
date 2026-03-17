import { useMutation, useQueryClient } from "@tanstack/react-query";
import { deleteImage, batchDeleteImages } from "../api/images";

export function useDeleteImages() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (ids: string[]) =>
      ids.length === 1
        ? deleteImage(ids[0])
        : batchDeleteImages(ids),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["images"] });
    },
  });
}
