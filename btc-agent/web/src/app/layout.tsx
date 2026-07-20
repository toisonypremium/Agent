import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";
const geistSans=Geist({variable:"--font-geist-sans",subsets:["latin"]});
const geistMono=Geist_Mono({variable:"--font-geist-mono",subsets:["latin"]});
export const metadata:Metadata={title:"Agent Control Plane | Trading Operations",description:"Read-only monitoring dashboard for the Agent V2 deterministic trading runtime.",robots:{index:false,follow:false}};
export default function RootLayout({children}:Readonly<{children:React.ReactNode}>){return <html lang="en" className={`${geistSans.variable} ${geistMono.variable}`}><body>{children}</body></html>}
