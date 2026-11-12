import { render } from 'solid-js/web';
import '../../assets/ui.css';
import App from './App';

const root = document.getElementById('app')!;
root.replaceChildren();
render(() => <App root={root} />, root);
