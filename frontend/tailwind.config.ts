import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{vue,ts,tsx}"],
  theme: {
    extend: {
      colors: {
        surface: "#FFFFFF",
        canvas: "#F8FAFC",
      },
      borderRadius: {
        button: "12px",
        card: "18px",
        dialog: "20px",
      },
      fontFamily: {
        sans: ["Inter", "HarmonyOS Sans", "PingFang SC", "Microsoft YaHei", "sans-serif"],
      },
    },
  },
  plugins: [],
} satisfies Config;
