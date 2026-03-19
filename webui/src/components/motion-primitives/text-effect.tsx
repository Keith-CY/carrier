import * as React from 'react';
import { AnimatePresence, motion, type TargetAndTransition, type Transition, type Variant, type Variants } from 'motion/react';
import { cn } from '@/lib/utils';

export type PresetType = 'blur' | 'fade-in-blur' | 'scale' | 'fade' | 'slide';
export type PerType = 'word' | 'char' | 'line';

export type TextEffectProps = {
  children: string;
  per?: PerType;
  as?: keyof React.JSX.IntrinsicElements;
  variants?: {
    container?: Variants;
    item?: Variants;
  };
  className?: string;
  preset?: PresetType;
  delay?: number;
  speedReveal?: number;
  speedSegment?: number;
  trigger?: boolean;
  onAnimationComplete?: () => void;
  onAnimationStart?: () => void;
  segmentWrapperClassName?: string;
  containerTransition?: Transition;
  segmentTransition?: Transition;
  style?: React.CSSProperties;
};

const defaultStaggerTimes: Record<PerType, number> = {
  char: 0.03,
  word: 0.05,
  line: 0.1,
};

const defaultContainerVariants: Variants = {
  hidden: { opacity: 0 },
  visible: {
    opacity: 1,
    transition: { staggerChildren: 0.05 },
  },
  exit: {
    transition: { staggerChildren: 0.05, staggerDirection: -1 },
  },
};

const defaultItemVariants: Variants = {
  hidden: { opacity: 0 },
  visible: { opacity: 1 },
  exit: { opacity: 0 },
};

const presetVariants: Record<PresetType, { container: Variants; item: Variants }> = {
  blur: {
    container: defaultContainerVariants,
    item: {
      hidden: { opacity: 0, filter: 'blur(12px)' },
      visible: { opacity: 1, filter: 'blur(0px)' },
      exit: { opacity: 0, filter: 'blur(12px)' },
    },
  },
  'fade-in-blur': {
    container: defaultContainerVariants,
    item: {
      hidden: { opacity: 0, y: 20, filter: 'blur(12px)' },
      visible: { opacity: 1, y: 0, filter: 'blur(0px)' },
      exit: { opacity: 0, y: 20, filter: 'blur(12px)' },
    },
  },
  scale: {
    container: defaultContainerVariants,
    item: {
      hidden: { opacity: 0, scale: 0 },
      visible: { opacity: 1, scale: 1 },
      exit: { opacity: 0, scale: 0 },
    },
  },
  fade: {
    container: defaultContainerVariants,
    item: {
      hidden: { opacity: 0 },
      visible: { opacity: 1 },
      exit: { opacity: 0 },
    },
  },
  slide: {
    container: defaultContainerVariants,
    item: {
      hidden: { opacity: 0, y: 20 },
      visible: { opacity: 1, y: 0 },
      exit: { opacity: 0, y: 20 },
    },
  },
};

const AnimationSegment = React.memo(function AnimationSegment({
  segment,
  variants,
  per,
  segmentWrapperClassName,
}: {
  segment: string;
  variants: Variants;
  per: PerType;
  segmentWrapperClassName?: string;
}) {
  const content =
    per === 'line' ? (
      <motion.span variants={variants} className="block">
        {segment}
      </motion.span>
    ) : per === 'word' ? (
      <motion.span aria-hidden="true" variants={variants} className="inline-block whitespace-pre">
        {segment}
      </motion.span>
    ) : (
      <motion.span className="inline-block whitespace-pre">
        {segment.split('').map((char, index) => (
          <motion.span key={`char-${index}`} aria-hidden="true" variants={variants} className="inline-block whitespace-pre">
            {char}
          </motion.span>
        ))}
      </motion.span>
    );

  if (!segmentWrapperClassName) return content;

  return (
    <span className={cn(per === 'line' ? 'block' : 'inline-block', segmentWrapperClassName)}>
      {content}
    </span>
  );
});

function splitText(text: string, per: PerType) {
  if (per === 'line') return text.split('\n');
  return text.split(/(\s+)/);
}

function hasTransition(variant?: Variant): variant is TargetAndTransition & { transition?: Transition } {
  return !!variant && typeof variant === 'object' && 'transition' in variant;
}

function withTransition(baseVariants: Variants, transition?: Transition & { exit?: Transition }): Variants {
  if (!transition) return baseVariants;
  const { exit: _exit, ...mainTransition } = transition;

  return {
    ...baseVariants,
    visible: {
      ...baseVariants.visible,
      transition: {
        ...(hasTransition(baseVariants.visible) ? baseVariants.visible.transition : {}),
        ...mainTransition,
      },
    },
    exit: {
      ...baseVariants.exit,
      transition: {
        ...(hasTransition(baseVariants.exit) ? baseVariants.exit.transition : {}),
        ...mainTransition,
        staggerDirection: -1,
      },
    },
  };
}

export function TextEffect({
  children,
  per = 'word',
  as = 'p',
  variants,
  className,
  preset = 'fade',
  delay = 0,
  speedReveal = 1,
  speedSegment = 1,
  trigger = true,
  onAnimationComplete,
  onAnimationStart,
  segmentWrapperClassName,
  containerTransition,
  segmentTransition,
  style,
}: TextEffectProps) {
  const segments = splitText(children, per);
  const MotionTag = motion[as as keyof typeof motion] as typeof motion.div;
  const presetVariant = presetVariants[preset] || {
    container: defaultContainerVariants,
    item: defaultItemVariants,
  };
  const stagger = defaultStaggerTimes[per] / speedReveal;
  const duration = 0.3 / speedSegment;
  const containerVariants = withTransition(
    variants?.container || presetVariant.container,
    containerTransition || { staggerChildren: stagger, delayChildren: delay },
  );
  const itemVariants = withTransition(
    variants?.item || presetVariant.item,
    segmentTransition || { duration, ease: 'easeOut' },
  );

  return (
    <AnimatePresence mode="wait">
      {trigger ? (
        <MotionTag
          key={children}
          aria-label={children}
          className={className}
          style={style}
          variants={containerVariants}
          initial="hidden"
          animate="visible"
          exit="exit"
          onAnimationStart={onAnimationStart}
          onAnimationComplete={onAnimationComplete}
        >
          {segments.map((segment, index) => (
            <AnimationSegment
              key={`${segment}-${index}`}
              segment={segment}
              variants={itemVariants}
              per={per}
              segmentWrapperClassName={segmentWrapperClassName}
            />
          ))}
        </MotionTag>
      ) : null}
    </AnimatePresence>
  );
}
