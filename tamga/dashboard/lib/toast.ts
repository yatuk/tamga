import { toast as sonnerToast } from "sonner";

function formatDescription(detail?: string): string | undefined {
  if (!detail) return undefined;
  return `// ${detail}`;
}

export const toast = {
  success(title: string, detail?: string) {
    sonnerToast.success(title, { description: formatDescription(detail) });
  },
  error(title: string, detail?: string) {
    sonnerToast.error(title, { description: formatDescription(detail) });
  },
  info(title: string, detail?: string) {
    sonnerToast.info(title, { description: formatDescription(detail) });
  },
  warning(title: string, detail?: string) {
    sonnerToast.warning(title, { description: formatDescription(detail) });
  },
  message(title: string, detail?: string) {
    sonnerToast(title, { description: formatDescription(detail) });
  },
  promise: sonnerToast.promise,
  dismiss: sonnerToast.dismiss,
};
