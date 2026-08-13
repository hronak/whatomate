import type { VariantProps } from "class-variance-authority"
import { cva } from "class-variance-authority"

export { default as Button } from "./Button.vue"

export const buttonVariants = cva(
  "btn-press inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-lg text-sm font-medium transition-all duration-200 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        // Gradient primary button with glow
        default: "bg-linear-to-r from-emerald-500 to-green-600 text-white shadow-lg shadow-emerald-500/25 hover:shadow-emerald-500/40 hover:from-emerald-600 hover:to-green-700",
        destructive:
          "bg-linear-to-r from-red-500 to-rose-600 text-white shadow-lg shadow-red-500/25 hover:shadow-red-500/40",
        // Glass outline for dark mode
        outline:
          "border border-white/10 bg-white/2 text-foreground hover:bg-white/6 hover:border-white/20 light:border-gray-200 light:bg-white hover:light:bg-gray-50",
        // Active/selected state - works in both dark and light modes
        active:
          "border border-primary/50 bg-primary text-primary-foreground light:bg-primary light:text-primary-foreground light:border-primary/50",
        secondary:
          "bg-white/6 text-foreground hover:bg-white/10 light:bg-gray-100 hover:light:bg-gray-200",
        // Subtle ghost with hover
        ghost: "text-muted-foreground hover:bg-white/6 hover:text-foreground hover:light:bg-gray-100",
        link: "text-primary underline-offset-4 hover:underline",
        // Glass variant for cards/panels
        glass: "bg-white/4 border border-white/8 text-foreground hover:bg-white/8 light:bg-gray-50 light:border-gray-200 hover:light:bg-gray-100",
      },
      size: {
        "default": "h-9 px-4 py-2",
        "xs": "h-7 rounded-md px-2",
        "sm": "h-8 rounded-lg px-3 text-xs",
        "lg": "h-11 rounded-lg px-8",
        "icon": "h-9 w-9",
        "icon-sm": "size-8",
        "icon-lg": "size-10",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
)

export type ButtonVariants = VariantProps<typeof buttonVariants>
