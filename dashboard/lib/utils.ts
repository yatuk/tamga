/**
 * Merges class names and resolves conflicting Tailwind utilities, last one
 * wins. One implementation for the whole app: the generated shadcn/ui
 * components import it from the package, everything else from here.
 */
export { cn } from "cn";
