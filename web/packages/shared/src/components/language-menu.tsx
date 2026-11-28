import { useState } from 'react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Globe2, Languages } from 'lucide-react';
import { Button } from '@shared/components/ui/button';
import { setAppLanguage } from '@shared/i18n/language';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@shared/components/ui/dropdown-menu';

// Each icon turns and fades as the other takes its place; reduced motion swaps them at once.
const glyphLayer = 'absolute inset-0 flex items-center justify-center transition-[opacity,rotate,scale] duration-[var(--dur-short)] ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none';

/** The globe at rest, turning into the languages icon while the menu is open. */
function LanguageGlyph({ active }: { active: boolean }) {
  return (
    <span className='relative flex size-4 items-center justify-center'>
      <span className={cn(glyphLayer, active && '-rotate-16 scale-88 opacity-0')}>
        <Globe2 aria-hidden='true' data-language-icon='globe' />
      </span>
      <span className={cn(glyphLayer, !active && 'rotate-16 scale-88 opacity-0')}>
        <Languages aria-hidden='true' data-language-icon='languages' />
      </span>
    </span>
  );
}

/** Chooses the interface language; the choice is remembered in this browser. */
export function LanguageMenu({ size = 'icon-sm' }: { size?: 'icon-sm' | 'icon-xs' }) {
  const { i18n, t } = useTranslation();
  const [open, setOpen] = useState(false);

  function selectLanguage(language: string) {
    if (language !== 'en' && language !== 'zh-CN') return;

    void setAppLanguage(language);
  }

  return (
    <DropdownMenu onOpenChange={setOpen} open={open}>
      <DropdownMenuTrigger
        render={(
          <Button
            aria-label={t('language.menu')}
            size={size}
            variant='ghost'
          />
        )}
      >
        <LanguageGlyph active={open} />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' sideOffset={8}>
        <DropdownMenuGroup>
          <DropdownMenuRadioGroup
            aria-label={t('language.label')}
            onValueChange={selectLanguage}
            value={i18n.resolvedLanguage ?? 'en'}
          >
            <DropdownMenuRadioItem closeOnClick value='en'>{t('language.english')}</DropdownMenuRadioItem>
            <DropdownMenuRadioItem closeOnClick value='zh-CN'>
              {t('language.simplifiedChinese')}
            </DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
