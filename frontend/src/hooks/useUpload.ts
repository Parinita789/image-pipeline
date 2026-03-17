import { useMutation, useQueryClient } from "@tanstack/react-query";
import { uploadImage } from "../api/images";

export function useUpload() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ file }: { file: File }) => {
      const idempotencyKey = crypto.randomUUID();
      return uploadImage(file, idempotencyKey);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["images"] });
    },
  });
}
