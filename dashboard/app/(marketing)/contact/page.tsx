"use client";

import Script from "next/script";
import { Suspense } from "react";
import { ContactBody } from "@/app/(marketing)/_components/marketing/contact/ContactBody";
import { HCAPTCHA_SITEKEY } from "@/app/(marketing)/_components/marketing/contact/contact-constants";

export default function ContactPage() {
  return (
    <Suspense fallback={<div className="mx-auto max-w-3xl px-6 py-20 text-sm text-zinc-600 dark:text-zinc-400">Loading…</div>}>
      {HCAPTCHA_SITEKEY ? (
        <Script
          src="https://js.hcaptcha.com/1/api.js?render=explicit"
          strategy="lazyOnload"
          async
          defer
        />
      ) : null}
      <ContactBody />
    </Suspense>
  );
}
