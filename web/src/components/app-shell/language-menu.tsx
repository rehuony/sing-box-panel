import { useTranslation } from 'react-i18next';
import { Globe2, Languages } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { motion, useReducedMotion } from 'motion/react';

import { setAppLanguage } from '@/i18n';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';

const LANGUAGE_MOTION_DURATION_SECONDS = 0.22;

function LanguageGlyph({ active }: { active: boolean }) {
  const shouldReduceMotion = useReducedMotion();

  if (shouldReduceMotion === true) {
    return (
      <span className='relative flex size-[0.9375rem] items-center justify-center' data-motion='reduced'>
        <Languages aria-hidden='true' data-language-icon='languages' />
      </span>
    );
  }

  const transition = {
    duration: LANGUAGE_MOTION_DURATION_SECONDS,
    ease: [0.22, 1, 0.36, 1] as const,
  };

  return (
    <span
      className='relative flex size-[0.9375rem] items-center justify-center'
      data-active={active ? 'true' : 'false'}
      data-motion='full'
    >
      <motion.span
        animate={{
          opacity: active ? 0 : 1,
          rotate: active ? -16 : 0,
          scale: active ? 0.88 : 1,
        }}
        className='absolute inset-0 flex items-center justify-center'
        initial={false}
        transition={transition}
      >
        <Globe2 aria-hidden='true' data-language-icon='globe' />
      </motion.span>
      <motion.span
        animate={{
          opacity: active ? 1 : 0,
          rotate: active ? 0 : 16,
          scale: active ? 1 : 0.88,
        }}
        className='absolute inset-0 flex items-center justify-center'
        initial={false}
        transition={transition}
      >
        <Languages aria-hidden='true' data-language-icon='languages' />
      </motion.span>
    </span>
  );
}

export function LanguageMenu() {
  const { i18n, t } = useTranslation();
  const [focused, setFocused] = useState(false);
  const [hovered, setHovered] = useState(false);
  const [open, setOpen] = useState(false);
  const [settling, setSettling] = useState(false);
  const settleTimerRef = useRef<number | null>(null);

  useEffect(() => () => {
    if (settleTimerRef.current !== null) window.clearTimeout(settleTimerRef.current);
  }, []);

  const active = !settling && (focused || hovered || open);

  function selectLanguage(language: string) {
    if (language !== 'en' && language !== 'zh-CN') return;

    void setAppLanguage(language).finally(() => {
      setSettling(true);
      if (settleTimerRef.current !== null) window.clearTimeout(settleTimerRef.current);
      settleTimerRef.current = window.setTimeout(() => {
        setSettling(false);
        settleTimerRef.current = null;
      }, LANGUAGE_MOTION_DURATION_SECONDS * 1_000);
    });
  }

  return (
    <DropdownMenu onOpenChange={setOpen} open={open}>
      <DropdownMenuTrigger
        render={(
          <Button
            aria-label={t('language.menu')}
            className='telemetry-language-button'
            onBlur={() => setFocused(false)}
            onFocus={() => setFocused(true)}
            onMouseEnter={() => setHovered(true)}
            onMouseLeave={() => setHovered(false)}
            size='icon-sm'
            variant='ghost'
          />
        )}
      >
        <LanguageGlyph active={active} />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' sideOffset={8}>
        <DropdownMenuGroup>
          <DropdownMenuLabel>{t('language.label')}</DropdownMenuLabel>
          <DropdownMenuRadioGroup
            onValueChange={selectLanguage}
            value={i18n.resolvedLanguage ?? 'en'}
          >
            <DropdownMenuRadioItem value='en'>{t('language.english')}</DropdownMenuRadioItem>
            <DropdownMenuRadioItem value='zh-CN'>
              {t('language.simplifiedChinese')}
            </DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
