import type { SVGProps } from 'react';
import { cn } from '@/lib/utils';

type CarrierHeaderLogoProps = SVGProps<SVGSVGElement> & {
  title?: string;
};

/**
 * Carrier header logo.
 *
 * Inspiration: a command ship releasing small task drones.
 * Original geometry (not a direct game asset copy).
 */
export function CarrierHeaderLogo({ className, title = 'Carrier', ...props }: CarrierHeaderLogoProps) {
  return (
    <svg
      viewBox="0 0 64 64"
      role="img"
      aria-label={title}
      className={cn('h-5 w-5', className)}
      {...props}
    >
      <g stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" fill="none">
        <path
          d="M32 6L45 13L52 26L45 39L32 58L19 39L12 26L19 13Z"
          fill="currentColor"
          fillOpacity="0.08"
          strokeWidth="2.5"
        />
        <path
          d="M32 12L40 17L45 26L40 35L32 48L24 35L19 26L24 17Z"
          fill="currentColor"
          fillOpacity="0.16"
          strokeWidth="2"
        />
        <circle cx="32" cy="26" r="4.5" fill="currentColor" fillOpacity="0.9" strokeWidth="1.4" />

        <path d="M32 31V39" strokeWidth="1.8" strokeOpacity="0.7" />
        <path d="M32 39L22 49" strokeWidth="1.8" strokeOpacity="0.55" />
        <path d="M32 39L42 49" strokeWidth="1.8" strokeOpacity="0.55" />

        <path d="M32 48L29.4 52.4L34.6 52.4Z" fill="currentColor" fillOpacity="0.9" strokeWidth="1" />
        <path d="M22 49L19.8 52.6L24.2 52.6Z" fill="currentColor" fillOpacity="0.85" strokeWidth="1" />
        <path d="M42 49L39.8 52.6L44.2 52.6Z" fill="currentColor" fillOpacity="0.85" strokeWidth="1" />
      </g>
    </svg>
  );
}
